package eino

import (
	"context"
	"errors"
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Runner struct {
	inner *adk.Runner
}

func NewRunner(inner *adk.Runner) *Runner {
	return &Runner{inner: inner}
}

func (r *Runner) Run(ctx context.Context, input domainmessage.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if r == nil || r.inner == nil {
		return nil, fmt.Errorf("runner is nil")
	}
	opts = opts.WithDefaults(opts.CheckpointID)
	runID := opts.RunID
	turnID := events.TurnID(runID, 1)
	emitter := events.NewEmitter(runID, turnID, opts.Sink)

	if err := emitter.Emit(events.AgentStart(runID)); err != nil {
		return nil, err
	}
	if err := emitter.Emit(events.TurnStart(runID, turnID)); err != nil {
		return nil, err
	}
	if !input.Message.IsEmpty() && input.Message.Role == domainmessage.RoleUser {
		userMessage := input.Message
		messageID := fmt.Sprintf("%s:user", turnID)
		userEvent := events.UserMessage(runID, turnID, messageID, userMessage)
		if err := emitter.Emit(userEvent); err != nil {
			return nil, err
		}
	}

	unknownTools := newUnknownToolRecorder()
	ctx = withUnknownToolRecorder(ctx, unknownTools)
	iter := r.inner.Run(ctx, adaptMessagesForRunner(input.AllMessages()), adk.WithCheckPointID(opts.CheckpointID))
	converter := newConverter(emitter, unknownTools)
	for {
		lastEvent, err := converter.drain(ctx, iter)
		if err != nil {
			_ = emitter.Emit(events.AgentError(runID, err))
			return &runtimeport.RunResult{LastEvent: converter.lastEvent()}, err
		}

		if lastEvent != nil && lastEvent.Action != nil && lastEvent.Action.Interrupted != nil {
			scope := converter.lastScope()
			var memberOrder *int
			if scope.CallID != "" {
				if order, ok := converter.identities.orderForID(scope.CallID); ok {
					memberOrder = intPtr(order)
				}
			}
			interrupts := adaptInterruptsFromRunner(lastEvent.Action.Interrupted.InterruptContexts, scope, memberOrder)
			if len(interrupts) > 0 && opts.InterruptHandler != nil {
				targets, handlerErr := opts.InterruptHandler(ctx, interrupts)
				if handlerErr != nil {
					_ = emitter.Emit(events.AgentError(runID, handlerErr))
					return &runtimeport.RunResult{LastEvent: converter.lastEvent()}, handlerErr
				}
				resumeIter, resumeErr := r.inner.ResumeWithParams(ctx, opts.CheckpointID, &adk.ResumeParams{Targets: targets})
				if resumeErr != nil {
					err := fmt.Errorf("resume failed: %w", resumeErr)
					_ = emitter.Emit(events.AgentError(runID, err))
					return &runtimeport.RunResult{LastEvent: converter.lastEvent()}, err
				}
				iter = resumeIter
				continue
			}
		}
		break
	}

	if err := emitter.Emit(events.TurnEnd(runID, turnID)); err != nil {
		return nil, err
	}
	if err := emitter.Emit(events.AgentEnd(runID)); err != nil {
		return nil, err
	}
	return &runtimeport.RunResult{LastEvent: converter.lastEvent()}, nil
}

func adaptInterruptsFromRunner(interrupts []*adk.InterruptCtx, scope MemberScope, memberOrder *int) []runtimeport.Interrupt {
	result := make([]runtimeport.Interrupt, 0, len(interrupts))
	for _, ic := range interrupts {
		info := ic.Info
		metadata := runtimeport.InterruptMetadata{}
		switch payload := info.(type) {
		case runtimeport.InterruptPayload:
			info = payload.Info
			metadata = payload.Metadata
		case *runtimeport.InterruptPayload:
			if payload != nil {
				info = payload.Info
				metadata = payload.Metadata
			}
		}
		next := runtimeport.Interrupt{
			ID:          ic.ID,
			IsRootCause: ic.IsRootCause,
			Info:        info,
		}
		if metadata.MemberCallID != "" {
			next.MemberCallID = metadata.MemberCallID
			next.MemberToolName = metadata.MemberToolName
			next.MemberName = metadata.MemberName
			next.MemberOrder = metadata.MemberOrder
		} else if scope.CallID != "" {
			next.MemberCallID = scope.CallID
			next.MemberToolName = scope.ToolName
			next.MemberName = scope.Name
			next.MemberOrder = memberOrder
		}
		if next.MemberCallID != "" && next.MemberOrder == nil {
			next.MemberOrder = memberOrder
		}
		result = append(result, next)
	}
	return result
}

type converter struct {
	emitter    *events.Emitter
	identities *toolIdentityTracker
	unknowns   *unknownToolRecorder
	scope      MemberScope
}

func newConverter(emitter *events.Emitter, unknowns *unknownToolRecorder) *converter {
	return &converter{
		emitter:    emitter,
		identities: newToolIdentityTracker(),
		unknowns:   unknowns,
	}
}

func (c *converter) emit(event events.Event) error {
	return c.emitter.Emit(event)
}

func (c *converter) lastEvent() events.Event {
	return c.emitter.LastEvent()
}

func (c *converter) lastScope() MemberScope {
	if c == nil {
		return MemberScope{}
	}
	return c.scope
}

func (c *converter) drain(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent]) (*adk.AgentEvent, error) {
	var lastEvent *adk.AgentEvent
	for {
		select {
		case <-ctx.Done():
			return lastEvent, ctx.Err()
		default:
		}

		if err := c.flushUnknownToolReports(); err != nil {
			return lastEvent, err
		}

		event, ok := iter.Next()
		if !ok {
			if err := c.flushUnknownToolReports(); err != nil {
				return lastEvent, err
			}
			return lastEvent, nil
		}
		lastEvent = event
		if err := c.flushUnknownToolReports(); err != nil {
			return lastEvent, err
		}
		if err := c.process(ctx, event); err != nil {
			return lastEvent, err
		}
	}
}

func (c *converter) flushUnknownToolReports() error {
	var pending []unknownToolReport
	for _, report := range c.unknowns.take() {
		if events.IsInternalToolName(report.ToolName) {
			continue
		}
		nEvent := events.ToolCallCompleted(events.ToolEvent{
			AgentName:  report.AgentName,
			ToolCallID: report.ToolCallID,
			ToolName:   report.ToolName,
			ToolArgs:   report.ToolArgs,
			ToolResult: normalizeToolResultContent(report.ToolResult),
		})
		report.Scope.apply(&nEvent, c)
		c.identities.attach(&nEvent, report.Scope)
		if nEvent.ToolCallID == "" || nEvent.ToolCallRef == "" {
			pending = append(pending, report)
			continue
		}
		if err := c.emit(nEvent); err != nil {
			return err
		}
	}
	for _, report := range pending {
		c.unknowns.add(report)
	}
	return nil
}

func (c *converter) process(ctx context.Context, event *adk.AgentEvent) error {
	scope, cleanupScope := consumeAgentEventScope(event)
	defer cleanupScope()
	c.scope = scope

	if event.Err != nil {
		if isContextCanceled(ctx, event.Err) {
			return nil
		}
		nEvent := events.Error(event.AgentName, formatRunPath(event.RunPath), event.Err)
		scope.apply(&nEvent, c)
		return c.emit(nEvent)
	}

	if event.Action != nil {
		if err := c.handleAction(event, scope); err != nil {
			return err
		}
	}

	if event.Output != nil && event.Output.MessageOutput != nil {
		return c.handleMessageOutput(ctx, event, scope)
	}
	return nil
}

func (c *converter) handleAction(event *adk.AgentEvent, scope MemberScope) error {
	action := event.Action
	if action.TransferToAgent != nil {
		content := fmt.Sprintf("Transfer to agent: %s", action.TransferToAgent.DestAgentName)
		nEvent := events.SystemNotice(event.AgentName, formatRunPath(event.RunPath), "transfer", content)
		scope.apply(&nEvent, c)
		return c.emit(nEvent)
	}
	if action.Interrupted != nil {
		return nil
	}
	if action.Exit {
		nEvent := events.Event{
			Type:      events.EventAgentCompleted,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Content:   "Agent execution completed",
		}
		scope.apply(&nEvent, c)
		return c.emit(nEvent)
	}
	return nil
}

func (c *converter) handleMessageOutput(ctx context.Context, event *adk.AgentEvent, scope MemberScope) error {
	msgOutput := event.Output.MessageOutput
	if msg := msgOutput.Message; msg != nil {
		return c.handleRegularMessage(event, msg, scope)
	}
	if stream := msgOutput.MessageStream; stream != nil {
		return c.handleStreamingMessage(ctx, event, stream, scope)
	}
	return nil
}

func (c *converter) handleRegularMessage(event *adk.AgentEvent, msg *schema.Message, scope MemberScope) error {
	if msg.Role == schema.Tool {
		if events.IsInternalToolName(msg.ToolName) || events.IsInternalContinueContent(msg.Content) {
			return nil
		}
		return c.emitToolResultMessage(event, msg, scope)
	}

	messageID := c.messageID(event, "assistant")
	message := adaptMessageFromRunner(msg)
	message.ToolCalls = c.ensureMessageToolIdentities(messageID, scope, message.ToolCalls)
	messageMeta := events.MessageEvent{
		MessageID: messageID,
		Role:      message.Role,
		AgentName: event.AgentName,
		RunPath:   formatRunPath(event.RunPath),
		Message:   &message,
	}
	start := events.AssistantStarted(messageMeta)
	scope.apply(&start, c)
	if err := c.emit(start); err != nil {
		return err
	}
	if msg.ReasoningContent != "" {
		deltaMeta := messageMeta
		deltaMeta.DeltaKind = domainevent.DeltaReasoning
		delta := events.AssistantDelta(deltaMeta, msg.ReasoningContent)
		scope.apply(&delta, c)
		if err := c.emit(delta); err != nil {
			return err
		}
	}
	if msg.Content != "" {
		deltaMeta := messageMeta
		deltaMeta.DeltaKind = domainevent.DeltaOutput
		delta := events.AssistantDelta(deltaMeta, msg.Content)
		scope.apply(&delta, c)
		if err := c.emit(delta); err != nil {
			return err
		}
	}
	endMeta := messageMeta
	endMeta.Content = msg.Content
	endMeta.ReasoningContent = msg.ReasoningContent
	endMeta.ToolCalls = message.ToolCalls
	endMeta.ToolCallRefs = c.identities.refsFor(message.ToolCalls)
	end := events.AssistantCompleted(endMeta)
	scope.apply(&end, c)
	if err := c.emit(end); err != nil {
		return err
	}
	return c.emitToolStarts(event, messageID, message.ToolCalls, scope)
}

func (c *converter) emitToolResultMessage(event *adk.AgentEvent, msg *schema.Message, scope MemberScope) error {
	content := normalizeToolResultContent(msg.Content)
	toolEvent := events.ToolCallCompleted(events.ToolEvent{
		AgentName:  event.AgentName,
		RunPath:    formatRunPath(event.RunPath),
		ToolCallID: msg.ToolCallID,
		ToolName:   msg.ToolName,
		ToolResult: content,
	})
	scope.apply(&toolEvent, c)
	c.identities.attach(&toolEvent, scope)
	return c.emit(toolEvent)
}

func (c *converter) emitToolStarts(event *adk.AgentEvent, sourceMessageID string, toolCalls []domainmessage.ToolCall, scope MemberScope) error {
	for position, tc := range toolCalls {
		if events.IsInternalToolName(tc.Function.Name) {
			continue
		}
		ref := c.identities.ensure(sourceMessageID, position, scope, &tc)
		c.identities.rememberResult(tc.Function.Name, tc.ID, scope)
		nEvent := events.ToolCallStarted(events.ToolEvent{
			AgentName:     event.AgentName,
			RunPath:       formatRunPath(event.RunPath),
			ToolCallID:    tc.ID,
			ToolCallRef:   ref,
			ToolName:      tc.Function.Name,
			ToolArgs:      tc.Function.Arguments,
			ToolCall:      &tc,
			ToolCallIndex: tc.Index,
		})
		scope.apply(&nEvent, c)
		if err := c.emit(nEvent); err != nil {
			return err
		}
	}
	return nil
}

type streamState struct {
	messageStarted bool
	messageID      string
	role           domainmessage.Role
	content        strings.Builder
	reasoning      strings.Builder
	toolCalls      map[int]domainmessage.ToolCall
	toolArgs       map[int]string
	toolRefs       map[int]string
	toolStarted    map[int]bool
	usage          *streamUsage
}

type streamUsage struct {
	promptTokens     int
	completionTokens int
	totalTokens      int
}

func newStreamState(event *adk.AgentEvent) *streamState {
	return &streamState{
		messageID:   fmt.Sprintf("msg_%d", atomic.AddInt64(&globalMessageSeq, 1)),
		role:        domainmessage.RoleAssistant,
		toolCalls:   make(map[int]domainmessage.ToolCall),
		toolArgs:    make(map[int]string),
		toolRefs:    make(map[int]string),
		toolStarted: make(map[int]bool),
	}
}

func (c *converter) handleStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream *schema.StreamReader[*schema.Message], scope MemberScope) error {
	ss := newStreamState(event)
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if isContextCanceled(ctx, err) {
				return nil
			}
			return err
		}
		if err := c.processStreamChunk(event, chunk, ss, scope); err != nil {
			return err
		}
	}
	if ss.messageStarted {
		message := domainmessage.Message{
			Role:             ss.role,
			Content:          ss.content.String(),
			ReasoningContent: ss.reasoning.String(),
		}
		indexes := make([]int, 0, len(ss.toolCalls))
		for idx := range ss.toolCalls {
			indexes = append(indexes, idx)
		}
		sort.Ints(indexes)
		for _, idx := range indexes {
			message.ToolCalls = append(message.ToolCalls, ss.toolCalls[idx])
		}
		end := events.AssistantCompleted(events.MessageEvent{
			MessageID:        ss.messageID,
			Role:             ss.role,
			AgentName:        event.AgentName,
			RunPath:          formatRunPath(event.RunPath),
			Message:          &message,
			Content:          message.Content,
			ReasoningContent: message.ReasoningContent,
			ToolCalls:        message.ToolCalls,
			ToolCallRefs:     ss.toolRefs,
		})
		attachStreamUsage(&end, ss)
		scope.apply(&end, c)
		if err := c.emit(end); err != nil {
			return err
		}
		if err := c.emitStreamUsage(event, ss, scope); err != nil {
			return err
		}
		if err := c.emitToolStarts(event, ss.messageID, message.ToolCalls, scope); err != nil {
			return err
		}
		return nil
	}
	if err := c.emitStreamUsage(event, ss, scope); err != nil {
		return err
	}
	return nil
}

func (c *converter) processStreamChunk(event *adk.AgentEvent, chunk *schema.Message, ss *streamState, scope MemberScope) error {
	if chunk == nil {
		return nil
	}
	if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
		usage := chunk.ResponseMeta.Usage
		ss.usage = &streamUsage{
			promptTokens:     usage.PromptTokens,
			completionTokens: usage.CompletionTokens,
			totalTokens:      usage.TotalTokens,
		}
	}
	if chunk.Role == schema.Tool {
		if events.IsInternalToolName(chunk.ToolName) || events.IsInternalContinueContent(chunk.Content) {
			return nil
		}
		nEvent := events.ToolCallResultDelta(events.ToolEvent{
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ToolCallID: chunk.ToolCallID,
			ToolName:   chunk.ToolName,
			Content:    chunk.Content,
		})
		scope.apply(&nEvent, c)
		c.identities.attach(&nEvent, scope)
		return c.emit(nEvent)
	}
	if !ss.messageStarted {
		ss.messageStarted = true
		if chunk.Role != "" {
			ss.role = adaptRoleFromRunner(chunk.Role)
		}
		start := events.AssistantStarted(events.MessageEvent{
			MessageID: ss.messageID,
			Role:      ss.role,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
		})
		scope.apply(&start, c)
		if err := c.emit(start); err != nil {
			return err
		}
	}
	if chunk.ReasoningContent != "" {
		ss.reasoning.WriteString(chunk.ReasoningContent)
		nEvent := events.AssistantDelta(events.MessageEvent{
			MessageID: ss.messageID,
			Role:      ss.role,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			DeltaKind: domainevent.DeltaReasoning,
		}, chunk.ReasoningContent)
		scope.apply(&nEvent, c)
		if err := c.emit(nEvent); err != nil {
			return err
		}
	}
	if chunk.Content != "" {
		ss.content.WriteString(chunk.Content)
		nEvent := events.AssistantDelta(events.MessageEvent{
			MessageID: ss.messageID,
			Role:      ss.role,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			DeltaKind: domainevent.DeltaOutput,
		}, chunk.Content)
		scope.apply(&nEvent, c)
		if err := c.emit(nEvent); err != nil {
			return err
		}
	}
	for _, tc := range chunk.ToolCalls {
		if events.IsInternalToolName(tc.Function.Name) {
			continue
		}
		idx := 0
		if tc.Index != nil {
			idx = *tc.Index
		}
		current := ss.toolCalls[idx]
		if current.ID == "" && tc.ID != "" {
			current.ID = tc.ID
		}
		current.Index = tc.Index
		current.Type = tc.Type
		if tc.Function.Name != "" {
			current.Function.Name = tc.Function.Name
		}
		current.Function.Arguments += tc.Function.Arguments
		ss.toolArgs[idx] += tc.Function.Arguments
		ref := c.identities.ensure(ss.messageID, idx, scope, &current)
		ss.toolCalls[idx] = current
		ss.toolRefs[idx] = ref
		if tc.Function.Arguments != "" {
			nEvent := events.AssistantDelta(events.MessageEvent{
				MessageID:   ss.messageID,
				Role:        ss.role,
				AgentName:   event.AgentName,
				RunPath:     formatRunPath(event.RunPath),
				DeltaKind:   domainevent.DeltaToolArgs,
				ToolCallID:  current.ID,
				ToolCallRef: ref,
				ToolName:    current.Function.Name,
			}, tc.Function.Arguments)
			nEvent.ToolCallIndex = current.Index
			scope.apply(&nEvent, c)
			if err := c.emit(nEvent); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *converter) emitStreamUsage(event *adk.AgentEvent, ss *streamState, scope MemberScope) error {
	if ss == nil || ss.usage == nil {
		return nil
	}
	usageEvent := events.Usage(event.AgentName, formatRunPath(event.RunPath), ss.usage.promptTokens, ss.usage.completionTokens, ss.usage.totalTokens)
	scope.apply(&usageEvent, c)
	ss.usage = nil
	return c.emit(usageEvent)
}

func attachStreamUsage(event *events.Event, ss *streamState) {
	if event == nil || ss == nil || ss.usage == nil {
		return
	}
	event.PromptTokens = ss.usage.promptTokens
	event.CompletionTokens = ss.usage.completionTokens
	event.TotalTokens = ss.usage.totalTokens
	event.Usage = &domainevent.UsagePayload{
		PromptTokens:     ss.usage.promptTokens,
		CompletionTokens: ss.usage.completionTokens,
		TotalTokens:      ss.usage.totalTokens,
	}
	ss.usage = nil
}

func (c *converter) messageID(event *adk.AgentEvent, suffix string) string {
	return fmt.Sprintf("msg_%s_%s_%d", event.AgentName, suffix, atomic.AddInt64(&globalMessageSeq, 1))
}

func (c *converter) ensureMessageToolIdentities(sourceMessageID string, scope MemberScope, toolCalls []domainmessage.ToolCall) []domainmessage.ToolCall {
	if len(toolCalls) == 0 {
		return toolCalls
	}
	result := make([]domainmessage.ToolCall, len(toolCalls))
	copy(result, toolCalls)
	for i := range result {
		c.identities.ensure(sourceMessageID, i, scope, &result[i])
	}
	return result
}

func formatRunPath(runPath []adk.RunStep) string {
	return fmt.Sprintf("%v", runPath)
}

func normalizeToolResultContent(content string) string {
	return content
}

func isContextCanceled(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded")
}

var globalMessageSeq int64

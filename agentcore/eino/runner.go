package eino

import (
	"context"
	"errors"
	"fkteams/agentcore"
	"fkteams/events"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

type Runner struct {
	inner *adk.Runner
}

func NewRunner(inner *adk.Runner) *Runner {
	return &Runner{inner: inner}
}

func (r *Runner) Run(ctx context.Context, input agentcore.TurnInput, opts agentcore.RunOptions) (*agentcore.RunResult, error) {
	if r == nil || r.inner == nil {
		return nil, fmt.Errorf("runner is nil")
	}
	if opts.Sink == nil {
		opts.Sink = func(agentcore.Event) error { return nil }
	}

	runID := opts.RunID
	if runID == "" {
		runID = opts.CheckpointID
	}
	turnID := fmt.Sprintf("%s:turn:1", runID)

	if err := opts.Sink(agentcore.Event{Type: agentcore.EventAgentStart, RunID: runID}); err != nil {
		return nil, err
	}
	if err := opts.Sink(agentcore.Event{Type: agentcore.EventTurnStart, RunID: runID, TurnID: turnID}); err != nil {
		return nil, err
	}
	if !input.Message.IsEmpty() && input.Message.Role == agentcore.RoleUser {
		userMessage := input.Message
		displayText := userMessage.DisplayText()
		messageID := fmt.Sprintf("%s:user", turnID)
		if err := opts.Sink(agentcore.Event{Type: agentcore.EventMessageStart, RunID: runID, TurnID: turnID, MessageID: messageID, Role: agentcore.RoleUser, Message: &userMessage, Content: displayText}); err != nil {
			return nil, err
		}
		if err := opts.Sink(agentcore.Event{Type: agentcore.EventMessageEnd, RunID: runID, TurnID: turnID, MessageID: messageID, Role: agentcore.RoleUser, Message: &userMessage, Content: displayText}); err != nil {
			return nil, err
		}
	}

	iter := r.inner.Run(ctx, adaptMessagesForRunner(input.AllMessages()), adk.WithCheckPointID(opts.CheckpointID))
	converter := newConverter(runID, turnID, opts.Sink)
	for {
		lastEvent, err := converter.drain(ctx, iter)
		if err != nil {
			_ = opts.Sink(agentcore.Event{Type: agentcore.EventAgentEnd, RunID: runID, Error: err.Error()})
			return &agentcore.RunResult{LastEvent: converter.lastEvent}, err
		}

		if lastEvent != nil && lastEvent.Action != nil && lastEvent.Action.Interrupted != nil {
			interrupts := adaptInterruptsFromRunner(lastEvent.Action.Interrupted.InterruptContexts)
			if len(interrupts) > 0 && opts.InterruptHandler != nil {
				targets, handlerErr := opts.InterruptHandler(ctx, interrupts)
				if handlerErr != nil {
					_ = opts.Sink(agentcore.Event{Type: agentcore.EventAgentEnd, RunID: runID, Error: handlerErr.Error()})
					return &agentcore.RunResult{LastEvent: converter.lastEvent}, handlerErr
				}
				resumeIter, resumeErr := r.inner.ResumeWithParams(ctx, opts.CheckpointID, &adk.ResumeParams{Targets: targets})
				if resumeErr != nil {
					err := fmt.Errorf("resume failed: %w", resumeErr)
					_ = opts.Sink(agentcore.Event{Type: agentcore.EventAgentEnd, RunID: runID, Error: err.Error()})
					return &agentcore.RunResult{LastEvent: converter.lastEvent}, err
				}
				iter = resumeIter
				continue
			}
		}
		break
	}

	if err := opts.Sink(agentcore.Event{Type: agentcore.EventTurnEnd, RunID: runID, TurnID: turnID}); err != nil {
		return nil, err
	}
	if err := opts.Sink(agentcore.Event{Type: agentcore.EventAgentEnd, RunID: runID}); err != nil {
		return nil, err
	}
	return &agentcore.RunResult{LastEvent: converter.lastEvent}, nil
}

func adaptInterruptsFromRunner(interrupts []*adk.InterruptCtx) []agentcore.Interrupt {
	result := make([]agentcore.Interrupt, 0, len(interrupts))
	for _, ic := range interrupts {
		result = append(result, agentcore.Interrupt{
			ID:          ic.ID,
			IsRootCause: ic.IsRootCause,
			Info:        ic.Info,
		})
	}
	return result
}

type converter struct {
	runID                    string
	turnID                   string
	sink                     agentcore.EventSink
	lastEvent                agentcore.Event
	toolRefsByID             sync.Map
	toolOrdersByID           sync.Map
	toolIDsByKey             sync.Map
	pendingToolResultsByName map[string][]string
	activeToolResultsByName  map[string]string
}

func newConverter(runID, turnID string, sink agentcore.EventSink) *converter {
	return &converter{
		runID:                    runID,
		turnID:                   turnID,
		sink:                     sink,
		pendingToolResultsByName: make(map[string][]string),
		activeToolResultsByName:  make(map[string]string),
	}
}

func (c *converter) emit(event agentcore.Event) error {
	event.RunID = firstNonEmpty(event.RunID, c.runID)
	event.TurnID = firstNonEmpty(event.TurnID, c.turnID)
	c.lastEvent = events.NormalizeEvent(event)
	if err := events.ValidateEventContract(c.lastEvent); err != nil {
		return err
	}
	return c.sink(c.lastEvent)
}

func (c *converter) drain(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent]) (*adk.AgentEvent, error) {
	var lastEvent *adk.AgentEvent
	for {
		select {
		case <-ctx.Done():
			return lastEvent, ctx.Err()
		default:
		}

		event, ok := iter.Next()
		if !ok {
			return lastEvent, nil
		}
		lastEvent = event
		if err := c.process(ctx, event); err != nil {
			return lastEvent, err
		}
	}
}

func (c *converter) process(ctx context.Context, event *adk.AgentEvent) error {
	scope, cleanupScope := consumeAgentEventScope(event)
	defer cleanupScope()

	if event.Err != nil {
		if isContextCanceled(ctx, event.Err) {
			return nil
		}
		nEvent := agentcore.Event{
			Type:      agentcore.EventError,
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Error:     event.Err.Error(),
		}
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
		nEvent := agentcore.Event{
			Type:       agentcore.EventAction,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: agentcore.ActionTransfer,
			Content:    fmt.Sprintf("Transfer to agent: %s", action.TransferToAgent.DestAgentName),
		}
		scope.apply(&nEvent, c)
		return c.emit(nEvent)
	}
	if action.Interrupted != nil {
		for _, ic := range action.Interrupted.InterruptContexts {
			content := fmt.Sprintf("%v", ic.Info)
			if stringer, ok := ic.Info.(fmt.Stringer); ok {
				content = stringer.String()
			}
			nEvent := agentcore.Event{
				Type:       agentcore.EventAction,
				AgentName:  event.AgentName,
				RunPath:    formatRunPath(event.RunPath),
				ActionType: agentcore.ActionInterrupted,
				Content:    content,
			}
			scope.apply(&nEvent, c)
			if err := c.emit(nEvent); err != nil {
				return err
			}
		}
	}
	if action.Exit {
		nEvent := agentcore.Event{
			Type:       agentcore.EventAction,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: agentcore.ActionExit,
			Content:    "Agent execution completed",
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
	start := agentcore.Event{Type: agentcore.EventMessageStart, MessageID: messageID, Role: message.Role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Message: &message}
	scope.apply(&start, c)
	if err := c.emit(start); err != nil {
		return err
	}
	if msg.ReasoningContent != "" {
		delta := agentcore.Event{Type: agentcore.EventMessageDelta, MessageID: messageID, Role: message.Role, DeltaKind: agentcore.DeltaReasoning, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Content: msg.ReasoningContent, Delta: msg.ReasoningContent}
		scope.apply(&delta, c)
		if err := c.emit(delta); err != nil {
			return err
		}
	}
	if msg.Content != "" {
		delta := agentcore.Event{Type: agentcore.EventMessageDelta, MessageID: messageID, Role: message.Role, DeltaKind: agentcore.DeltaOutput, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Content: msg.Content, Delta: msg.Content}
		scope.apply(&delta, c)
		if err := c.emit(delta); err != nil {
			return err
		}
	}
	end := agentcore.Event{Type: agentcore.EventMessageEnd, MessageID: messageID, Role: message.Role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Message: &message, Content: msg.Content, ReasoningContent: msg.ReasoningContent, ToolCalls: message.ToolCalls, ToolCallRefs: c.toolRefsForToolCalls(message.ToolCalls)}
	scope.apply(&end, c)
	if err := c.emit(end); err != nil {
		return err
	}
	return c.emitToolStarts(event, messageID, message.ToolCalls, scope)
}

func (c *converter) emitToolResultMessage(event *adk.AgentEvent, msg *schema.Message, scope MemberScope) error {
	content := normalizeToolResultContent(msg.Content)
	toolEvent := agentcore.Event{
		Type:       agentcore.EventToolEnd,
		AgentName:  event.AgentName,
		RunPath:    formatRunPath(event.RunPath),
		ToolCallID: msg.ToolCallID,
		ToolName:   msg.ToolName,
		Content:    content,
		ToolResult: content,
	}
	c.attachToolIdentity(&toolEvent)
	scope.apply(&toolEvent, c)
	if err := c.emit(toolEvent); err != nil {
		return err
	}

	message := adaptMessageFromRunner(msg)
	message.ToolCallID = toolEvent.ToolCallID
	message.Content = content
	messageID := c.messageID(event, "tool")
	start := agentcore.Event{Type: agentcore.EventMessageStart, MessageID: messageID, Role: agentcore.RoleTool, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Message: &message, ToolCallID: toolEvent.ToolCallID, ToolCallRef: toolEvent.ToolCallRef, ToolName: msg.ToolName}
	c.attachToolIdentity(&start)
	scope.apply(&start, c)
	if err := c.emit(start); err != nil {
		return err
	}
	end := agentcore.Event{Type: agentcore.EventMessageEnd, MessageID: messageID, Role: agentcore.RoleTool, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Message: &message, ToolCallID: toolEvent.ToolCallID, ToolCallRef: toolEvent.ToolCallRef, ToolName: msg.ToolName, Content: content}
	c.attachToolIdentity(&end)
	scope.apply(&end, c)
	return c.emit(end)
}

func (c *converter) emitToolStarts(event *adk.AgentEvent, sourceMessageID string, toolCalls []agentcore.ToolCall, scope MemberScope) error {
	for position, tc := range toolCalls {
		if events.IsInternalToolName(tc.Function.Name) {
			continue
		}
		ref := c.ensureToolIdentity(sourceMessageID, position, scope, &tc)
		c.rememberPendingToolResult(tc.Function.Name, tc.ID)
		nEvent := agentcore.Event{
			Type:        agentcore.EventToolStart,
			AgentName:   event.AgentName,
			RunPath:     formatRunPath(event.RunPath),
			ToolCallID:  tc.ID,
			ToolCallRef: ref,
			ToolName:    tc.Function.Name,
			ToolArgs:    tc.Function.Arguments,
			Content:     tc.Function.Arguments,
			ToolCall:    &tc,
		}
		if tc.Index != nil {
			nEvent.ToolCallIndex = tc.Index
		}
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
	role           agentcore.MessageRole
	content        strings.Builder
	reasoning      strings.Builder
	toolCalls      map[int]agentcore.ToolCall
	toolArgs       map[int]string
	toolRefs       map[int]string
	toolStarted    map[int]bool
}

func newStreamState(event *adk.AgentEvent) *streamState {
	return &streamState{
		messageID:   fmt.Sprintf("msg_%d", atomic.AddInt64(&globalMessageSeq, 1)),
		role:        agentcore.RoleAssistant,
		toolCalls:   make(map[int]agentcore.ToolCall),
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
		message := agentcore.Message{
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
		end := agentcore.Event{Type: agentcore.EventMessageEnd, MessageID: ss.messageID, Role: ss.role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), Message: &message, Content: message.Content, ReasoningContent: message.ReasoningContent, ToolCalls: message.ToolCalls, ToolCallRefs: ss.toolRefs}
		scope.apply(&end, c)
		if err := c.emit(end); err != nil {
			return err
		}
		if err := c.emitToolStarts(event, ss.messageID, message.ToolCalls, scope); err != nil {
			return err
		}
	}
	return nil
}

func (c *converter) processStreamChunk(event *adk.AgentEvent, chunk *schema.Message, ss *streamState, scope MemberScope) error {
	if chunk == nil {
		return nil
	}
	if chunk.ResponseMeta != nil && chunk.ResponseMeta.Usage != nil {
		usage := chunk.ResponseMeta.Usage
		usageEvent := agentcore.Event{
			Type:             agentcore.EventUsage,
			AgentName:        event.AgentName,
			RunPath:          formatRunPath(event.RunPath),
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
		scope.apply(&usageEvent, c)
		if err := c.emit(usageEvent); err != nil {
			return err
		}
	}
	if chunk.Role == schema.Tool {
		if events.IsInternalToolName(chunk.ToolName) || events.IsInternalContinueContent(chunk.Content) {
			return nil
		}
		nEvent := agentcore.Event{
			Type:       agentcore.EventToolUpdate,
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ToolCallID: chunk.ToolCallID,
			ToolName:   chunk.ToolName,
			Content:    chunk.Content,
			Delta:      chunk.Content,
			DeltaKind:  agentcore.DeltaToolResult,
		}
		c.attachToolIdentity(&nEvent)
		scope.apply(&nEvent, c)
		return c.emit(nEvent)
	}
	if !ss.messageStarted {
		ss.messageStarted = true
		if chunk.Role != "" {
			ss.role = adaptRoleFromRunner(chunk.Role)
		}
		start := agentcore.Event{Type: agentcore.EventMessageStart, MessageID: ss.messageID, Role: ss.role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath)}
		scope.apply(&start, c)
		if err := c.emit(start); err != nil {
			return err
		}
	}
	if chunk.ReasoningContent != "" {
		ss.reasoning.WriteString(chunk.ReasoningContent)
		nEvent := agentcore.Event{Type: agentcore.EventMessageDelta, MessageID: ss.messageID, Role: ss.role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), DeltaKind: agentcore.DeltaReasoning, Content: chunk.ReasoningContent, Delta: chunk.ReasoningContent}
		scope.apply(&nEvent, c)
		if err := c.emit(nEvent); err != nil {
			return err
		}
	}
	if chunk.Content != "" {
		ss.content.WriteString(chunk.Content)
		nEvent := agentcore.Event{Type: agentcore.EventMessageDelta, MessageID: ss.messageID, Role: ss.role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), DeltaKind: agentcore.DeltaOutput, Content: chunk.Content, Delta: chunk.Content}
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
		ref := c.ensureToolIdentity(ss.messageID, idx, scope, &current)
		ss.toolCalls[idx] = current
		ss.toolRefs[idx] = ref
		if tc.Function.Arguments != "" {
			nEvent := agentcore.Event{Type: agentcore.EventMessageDelta, MessageID: ss.messageID, Role: ss.role, AgentName: event.AgentName, RunPath: formatRunPath(event.RunPath), DeltaKind: agentcore.DeltaToolArgs, Content: tc.Function.Arguments, Delta: tc.Function.Arguments, ToolCallID: current.ID, ToolCallRef: ref, ToolName: current.Function.Name, ToolCallIndex: current.Index}
			scope.apply(&nEvent, c)
			if err := c.emit(nEvent); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *converter) messageID(event *adk.AgentEvent, suffix string) string {
	return fmt.Sprintf("msg_%s_%s_%d", event.AgentName, suffix, atomic.AddInt64(&globalMessageSeq, 1))
}

func (c *converter) ensureMessageToolIdentities(sourceMessageID string, scope MemberScope, toolCalls []agentcore.ToolCall) []agentcore.ToolCall {
	if len(toolCalls) == 0 {
		return toolCalls
	}
	result := make([]agentcore.ToolCall, len(toolCalls))
	copy(result, toolCalls)
	for i := range result {
		c.ensureToolIdentity(sourceMessageID, i, scope, &result[i])
	}
	return result
}

func (c *converter) toolRefsForToolCalls(toolCalls []agentcore.ToolCall) map[int]string {
	if len(toolCalls) == 0 {
		return nil
	}
	refs := make(map[int]string, len(toolCalls))
	for position, tc := range toolCalls {
		if tc.ID == "" {
			continue
		}
		ref, ok := c.toolRefsByID.Load(tc.ID)
		if !ok {
			continue
		}
		value, ok := ref.(string)
		if !ok || value == "" {
			continue
		}
		key := position
		if tc.Index != nil {
			key = *tc.Index
		}
		refs[key] = value
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func (c *converter) ensureToolIdentity(sourceMessageID string, position int, scope MemberScope, tc *agentcore.ToolCall) string {
	if tc == nil {
		return ""
	}
	if tc.ID == "" {
		key := c.syntheticToolKey(sourceMessageID, position, scope, tc)
		if existing, ok := c.toolIDsByKey.Load(key); ok {
			if value, ok := existing.(string); ok && value != "" {
				tc.ID = value
			}
		}
		if tc.ID == "" {
			tc.ID = "fk_tool_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			c.toolIDsByKey.Store(key, tc.ID)
		}
	}
	ref := "tool_call:" + tc.ID
	c.toolRefsByID.Store(tc.ID, ref)
	if tc.Index != nil {
		c.toolOrdersByID.Store(tc.ID, *tc.Index)
	}
	return ref
}

func (c *converter) syntheticToolKey(sourceMessageID string, position int, scope MemberScope, tc *agentcore.ToolCall) string {
	idx := position
	if tc != nil && tc.Index != nil {
		idx = *tc.Index
	}
	parts := []string{sourceMessageID, fmt.Sprintf("idx:%d", idx)}
	if scope.CallID != "" {
		parts = append(parts, "member:"+scope.CallID)
	}
	return strings.Join(parts, "|")
}

func (c *converter) attachToolIdentity(event *agentcore.Event) {
	if event == nil {
		return
	}
	if event.ToolCallID == "" && event.ToolName != "" {
		event.ToolCallID = c.toolResultIDByName(event.ToolName, event.Type == agentcore.EventToolEnd)
	}
	if event.ToolCallID == "" {
		return
	}
	if ref, ok := c.toolRefsByID.Load(event.ToolCallID); ok {
		if value, ok := ref.(string); ok && value != "" {
			event.ToolCallRef = value
		}
	}
	if order, ok := c.toolOrdersByID.Load(event.ToolCallID); ok {
		if value, ok := order.(int); ok {
			event.ToolCallIndex = &value
		}
	}
	if event.Type == agentcore.EventToolEnd && event.ToolName != "" {
		delete(c.activeToolResultsByName, event.ToolName)
		c.consumePendingToolResult(event.ToolName, event.ToolCallID)
	}
}

func (c *converter) rememberPendingToolResult(name, id string) {
	if name == "" || id == "" {
		return
	}
	c.pendingToolResultsByName[name] = append(c.pendingToolResultsByName[name], id)
}

func (c *converter) toolResultIDByName(name string, done bool) string {
	if name == "" {
		return ""
	}
	if id := c.activeToolResultsByName[name]; id != "" {
		if done {
			delete(c.activeToolResultsByName, name)
			c.consumePendingToolResult(name, id)
		}
		return id
	}
	queue := c.pendingToolResultsByName[name]
	if len(queue) == 0 {
		return ""
	}
	id := queue[0]
	if done {
		c.pendingToolResultsByName[name] = queue[1:]
	} else {
		c.activeToolResultsByName[name] = id
	}
	return id
}

func (c *converter) consumePendingToolResult(name, id string) {
	queue := c.pendingToolResultsByName[name]
	if len(queue) == 0 {
		return
	}
	if queue[0] == id {
		c.pendingToolResultsByName[name] = queue[1:]
		return
	}
	for i, queued := range queue {
		if queued != id {
			continue
		}
		c.pendingToolResultsByName[name] = append(queue[:i], queue[i+1:]...)
		return
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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

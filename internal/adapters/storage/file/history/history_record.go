package eventlog

import (
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/domain/message"

	"fkteams/internal/runtime/events"

	"sort"
	"strings"

	"time"
)

func NewHistoryRecorder() *HistoryRecorder {
	return &HistoryRecorder{
		activeMessages: make(map[string]*activeMessageContext),
		activeOrder:    make([]string, 0),
		messages:       make([]AgentMessage, 0),
		subagents:      make(map[string]*subagentRun),
		agentToolCalls: make(map[string]pendingToolCall),
	}
}

// truncateErrorContent 截断过长的错误内容，保留头尾部分
func truncateErrorContent(s string) string {
	runes := []rune(s)
	if len(runes) <= maxErrorContentLen {
		return s
	}
	head := maxErrorContentLen * 2 / 3
	tail := maxErrorContentLen - head
	return string(runes[:head]) + "\n...(truncated)...\n" + string(runes[len(runes)-tail:])
}

func toolResultKey(event Event) string {
	if event.ToolCallRef != "" {
		return event.ToolCallRef
	}
	if event.ToolCallID != "" {
		return "tool_call:" + event.ToolCallID
	}
	return ""
}

func toolResultContentFromEvent(event Event) string {
	content := event.ToolResult
	if content == "" {
		content = event.Content
	}
	return content
}

func eventMatchesPendingToolCall(tc pendingToolCall, event Event) bool {
	if tc.Ref != "" && event.ToolCallRef != "" && tc.Ref == event.ToolCallRef {
		return true
	}
	return tc.ID != "" && event.ToolCallID != "" && tc.ID == event.ToolCallID
}

func eventMatchesToolCallRecord(record *ToolCallRecord, event Event) bool {
	if record == nil {
		return false
	}
	if record.Ref != "" && event.ToolCallRef != "" && record.Ref == event.ToolCallRef {
		return true
	}
	return record.ID != "" && event.ToolCallID != "" && record.ID == event.ToolCallID
}

func pendingToolCallMatchesResultKey(tc pendingToolCall, resultKey string) bool {
	if resultKey == "" {
		return false
	}
	if tc.Ref != "" && tc.Ref == resultKey {
		return true
	}
	return tc.ID != "" && resultKey == "tool_call:"+tc.ID
}

func (h *HistoryRecorder) recordToolResult(ctx *activeMessageContext, event Event, content string) {
	if ctx == nil || content == "" || events.IsInternalContinueContent(content) {
		return
	}
	resultKey := toolResultKey(event)
	if resultKey == "" {
		return
	}
	idx := -1
	for i := range ctx.pendingToolCalls {
		if eventMatchesPendingToolCall(ctx.pendingToolCalls[i], event) {
			idx = i
			break
		}
	}
	if idx >= 0 {
		tc := ctx.pendingToolCalls[idx]
		ctx.pendingToolCalls = append(ctx.pendingToolCalls[:idx], ctx.pendingToolCalls[idx+1:]...)
		if events.IsInternalToolName(tc.Name) {
			return
		}
		if !h.updateToolCallEvent(ctx, tc, content) {
			ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
				Type:      MsgTypeToolCall,
				EventID:   tc.EventID,
				Sequence:  tc.Sequence,
				CreatedAt: tc.CreatedAt,
				RunID:     tc.RunID,
				TurnID:    tc.TurnID,
				MessageID: tc.MessageID,
				ToolCall:  ptrToolCallRecord(toolCallRecordFromPending(tc, content)),
			})
		}
		return
	}
	for i := range ctx.msg.Events {
		evt := &ctx.msg.Events[i]
		if evt.Type != MsgTypeToolCall || !eventMatchesToolCallRecord(evt.ToolCall, event) {
			continue
		}
		if evt.ToolCall.Result == "" {
			evt.ToolCall.Result = content
		}
		return
	}
	if event.ToolName == "" {
		return
	}
	var extra map[string]any
	if event.ToolCall != nil {
		extra = event.ToolCall.Extra
	}
	pending := h.pendingToolCallFromEvent(event, resultKey, event.ToolCallID, event.ToolCallIndex, event.ToolName, event.ToolArgs, extra)
	if !events.IsInternalToolName(pending.Name) {
		ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
			Type:      MsgTypeToolCall,
			EventID:   event.EventID,
			Sequence:  event.Sequence,
			CreatedAt: event.CreatedAt,
			RunID:     event.RunID,
			TurnID:    event.TurnID,
			MessageID: event.MessageID,
			ToolCall:  ptrToolCallRecord(toolCallRecordFromPending(pending, content)),
		})
	}
}

func historyActiveKey(event Event) string {
	if event.MemberCallID != "" {
		return "member:" + event.MemberCallID
	}
	return "agent:" + event.AgentName + "|" + event.RunPath
}

func activeMessageOrder(event Event) int {
	if event.MemberOrder != nil {
		return *event.MemberOrder
	}
	return 1_000_000
}

func (h *HistoryRecorder) ensureMessageContext(event Event) *activeMessageContext {
	if h.activeMessages == nil {
		h.activeMessages = make(map[string]*activeMessageContext)
	}
	key := historyActiveKey(event)
	if key == "agent:|" {
		key = "agent:" + event.AgentName
	}
	if ctx := h.activeMessages[key]; ctx != nil {
		return ctx
	}
	ctx := &activeMessageContext{
		msg: AgentMessage{
			AgentName:      event.AgentName,
			RunPath:        event.RunPath,
			MemberCallID:   event.MemberCallID,
			MemberToolName: event.MemberToolName,
			MemberName:     event.MemberName,
			StartTime:      time.Now(),
			Events:         make([]MessageEvent, 0),
		},
		toolResultChunks: make(map[string]string),
		order:            activeMessageOrder(event),
		createdSeq:       event.Sequence,
	}
	h.activeMessages[key] = ctx
	h.activeOrder = append(h.activeOrder, key)
	return ctx
}

func memberCallKeys(id string) []string {
	if id == "" {
		return nil
	}
	keys := []string{id}
	if strings.HasPrefix(id, "tool_call:") {
		keys = append(keys, strings.TrimPrefix(id, "tool_call:"))
	} else {
		keys = append(keys, "tool_call:"+id)
	}
	return keys
}

func (h *HistoryRecorder) rememberAgentToolCall(tc pendingToolCall) {
	if h.agentToolCalls == nil {
		h.agentToolCalls = make(map[string]pendingToolCall)
	}
	for _, key := range memberCallKeys(tc.ID) {
		h.agentToolCalls[key] = tc
	}
	for _, key := range memberCallKeys(tc.Ref) {
		h.agentToolCalls[key] = tc
	}
	if tc.Name != "" {
		h.agentToolCalls[tc.Name] = tc
	}
}

func (h *HistoryRecorder) ensureSubagentRun(event Event) *subagentRun {
	if event.MemberCallID == "" {
		return nil
	}
	if h.subagents == nil {
		h.subagents = make(map[string]*subagentRun)
	}
	for _, key := range memberCallKeys(event.MemberCallID) {
		if run := h.subagents[key]; run != nil {
			return run
		}
	}
	agentRunID := newPrefixedID("agent")
	agentName := event.MemberName
	if agentName == "" {
		agentName = event.AgentName
	}
	run := &subagentRun{
		AgentRunID:       agentRunID,
		ParentToolCallID: event.MemberCallID,
		ToolName:         event.MemberToolName,
		AgentName:        agentName,
		TranscriptPath:   subagentTranscriptPath(h.sessionDir, agentRunID),
	}
	if err := writeSubagentMetadata(h.sessionDir, SubagentMetadata{
		AgentRunID:   agentRunID,
		Agent:        agentName,
		ParentCallID: event.MemberCallID,
		ToolName:     event.MemberToolName,
	}); err != nil && h.persistErr == nil {
		h.persistErr = err
	}
	for _, key := range memberCallKeys(event.MemberCallID) {
		h.subagents[key] = run
	}
	return run
}

func (h *HistoryRecorder) recordTranscript(event Event) {
	var target *subagentRun
	if event.MemberCallID != "" {
		target = h.ensureSubagentRun(event)
	}
	agent := event.AgentName
	if target != nil {
		agent = target.AgentName
	}
	ts := event.CreatedAt
	if ts.IsZero() {
		ts = time.Now()
	}
	base := TranscriptEvent{
		At:    ts,
		Agent: agent,
	}

	switch event.Type {
	case events.EventUserMessage:
		content := event.Content
		parts := []message.ContentPart(nil)
		if event.Message != nil {
			content = event.Message.DisplayText()
			parts = append(parts, event.Message.ContentParts...)
		}
		base.Type = TranscriptUserMessage
		base.Agent = ""
		base.Content = content
		base.ContentParts = parts
		h.appendTranscriptEvent(base, target)
	case events.EventAssistantReasoning, events.EventAssistantText:
		h.bufferAssistantMessage(event)
		return
	case events.EventAssistantCompleted:
		if event.Role == message.RoleTool || event.Role == message.RoleUser {
			return
		}
		ctx := h.ensureMessageContext(event)
		delete(ctx.pendingAssistant, assistantMessageKey{event.RunID, event.MessageID})
		content := event.Content
		reasoning := event.ReasoningContent
		parts := []message.ContentPart(nil)
		if event.Message != nil {
			content = event.Message.DisplayText()
			reasoning = event.Message.ReasoningContent
			parts = append(parts, event.Message.ContentParts...)
		}
		base.Usage = usageRecordFromEvent(event)
		if content == "" && reasoning == "" && len(parts) == 0 && base.Usage == nil {
			return
		}
		base.Type = TranscriptAssistantMessage
		if content == "" {
			base.Type = TranscriptAgentStep
		}
		base.Content = content
		base.Reasoning = reasoning
		base.ContentParts = parts
		h.appendTranscriptEvent(base, target)
	case EventToolCallStarted:
		toolCalls := events.ToolCallsFromEvent(event)
		for i, tc := range toolCalls {
			if tc.Function.Name == "" || events.IsInternalToolName(tc.Function.Name) {
				continue
			}
			ref := events.ToolCallRefAt(event, tc, i)
			pending := h.pendingToolCallFromEvent(event, ref, tc.ID, tc.Index, tc.Function.Name, tc.Function.Arguments, tc.Extra)
			record := toolCallRecordFromPending(pending, "")
			line := base
			line.Type = TranscriptToolCallStart
			line.CallID = pending.ID
			line.Name = record.Name
			line.Args = record.Arguments
			line.Extra = message.CloneMetadata(record.Extra)
			line.Display = record.DisplayName
			line.Kind = record.Kind
			line.Target = record.Target
			h.appendTranscriptEvent(line, target)
			if record.Kind == toolmeta.ToolKindAgent {
				h.rememberAgentToolCall(pending)
			}
		}
	case EventToolCallCompleted:
		content := toolResultContentFromEvent(event)
		if events.IsInternalContinueContent(content) {
			return
		}
		toolName := event.ToolName
		if toolName == "" && event.ToolCall != nil {
			toolName = event.ToolCall.Function.Name
		}
		resultPayload := h.toolResultPayload(toolName, content)
		line := base
		line.Type = TranscriptToolCallEnd
		line.CallID = event.ToolCallID
		line.Result = resultPayload.Result
		line.ResultRef = resultPayload.ResultRef
		line.Summary = resultPayload.Summary
		line.Truncated = resultPayload.Truncated
		line.OriginalChars = resultPayload.OriginalChars
		h.appendTranscriptEvent(line, target)
	case EventUsageReported:
		return
	case events.EventAskRequested, events.EventAskAnswered:
		base.Type = TranscriptAskRequested
		if event.Type == events.EventAskAnswered {
			base.Type = TranscriptAskAnswered
		}
		record := &AskRecord{}
		if event.Ask != nil {
			record.ID = event.Ask.ID
			record.Question = event.Ask.Question
			record.Options = append([]string(nil), event.Ask.Options...)
			record.MultiSelect = event.Ask.MultiSelect
			record.Selected = append([]string(nil), event.Ask.Selected...)
			record.FreeText = event.Ask.FreeText
		}
		if event.Type == events.EventAskAnswered {
			record.Answered = true
		}
		base.Ask = record
		base.Content = event.Content
		h.appendTranscriptEvent(base, target)
	case EventSystemNotice:
		base.Type = TranscriptSystemNotice
		base.Content = event.Content
		base.Detail = event.Detail
		h.appendTranscriptEvent(base, target)
	case EventError:
		friendly := events.NormalizeFriendlyError(event.Error)
		friendly.TechnicalDetail = truncateErrorContent(friendly.TechnicalDetail)
		historyError := FriendlyError(friendly)
		base.Type = TranscriptError
		base.Content = historyError.Message
		base.Error = &historyError
		h.appendTranscriptEvent(base, target)
	case events.EventCancelled:
		base.Type = TranscriptCancelled
		base.Content = event.Content
		h.appendTranscriptEvent(base, target)
	}
}

// bufferAssistantMessage 保留未收到完整消息的增量，供取消或失败时定稿。
func (h *HistoryRecorder) bufferAssistantMessage(event Event) {
	if event.Content == "" || event.Role == message.RoleTool || event.Role == message.RoleUser {
		return
	}
	ctx := h.ensureMessageContext(event)
	if ctx.pendingAssistant == nil {
		ctx.pendingAssistant = make(map[assistantMessageKey]pendingAssistantMessage)
	}
	key := assistantMessageKey{event.RunID, event.MessageID}
	pending, exists := ctx.pendingAssistant[key]
	if !exists {
		pending = pendingAssistantMessage{event: event, firstEvent: len(ctx.msg.Events)}
		pending.event.Type = events.EventAssistantCompleted
		pending.event.Content = ""
		pending.event.ReasoningContent = ""
		pending.event.Message = nil
		ctx.assistantOrder = append(ctx.assistantOrder, key)
	}
	if event.Type == events.EventAssistantReasoning {
		pending.event.ReasoningContent += event.Content
	} else {
		pending.event.Content += event.Content
	}
	ctx.pendingAssistant[key] = pending
}

func (h *HistoryRecorder) flushAssistantMessages(ctx *activeMessageContext) {
	for _, key := range ctx.assistantOrder {
		if pending, ok := ctx.pendingAssistant[key]; ok {
			h.recordTranscript(pending.event)
		}
	}
	ctx.assistantOrder = nil
}

// completeAssistantMessage 让内存与落盘投影都以完整消息为准，也支持没有增量的非流式执行。
func (h *HistoryRecorder) completeAssistantMessage(event Event) {
	ctx := h.ensureMessageContext(event)
	key := assistantMessageKey{event.RunID, event.MessageID}
	pending, exists := ctx.pendingAssistant[key]
	start := len(ctx.msg.Events)
	if exists {
		start = pending.firstEvent
	}
	content, reasoning := event.Content, event.ReasoningContent
	var parts []message.ContentPart
	if event.Message != nil {
		content = event.Message.DisplayText()
		reasoning = event.Message.ReasoningContent
		parts = append(parts, event.Message.ContentParts...)
	}
	replacement := make([]MessageEvent, 0, 3)
	if reasoning != "" {
		evt := historyEventEnvelope(event, MsgTypeReasoning)
		evt.Content = reasoning
		replacement = append(replacement, evt)
	}
	if content != "" || len(parts) > 0 {
		evt := historyEventEnvelope(event, MsgTypeText)
		evt.Content, evt.ContentParts = content, parts
		replacement = append(replacement, evt)
	}
	if usage := usageRecordFromEvent(event); usage != nil {
		evt := historyEventEnvelope(event, MsgTypeUsageReported)
		evt.Usage = usage
		replacement = append(replacement, evt)
	}
	updated := append([]MessageEvent(nil), ctx.msg.Events[:start]...)
	updated = append(updated, replacement...)
	positions := make([]int, len(ctx.msg.Events)+1)
	for i, evt := range ctx.msg.Events {
		if i < start {
			positions[i] = i
			continue
		}
		positions[i] = len(updated)
		if evt.RunID == event.RunID && evt.MessageID == event.MessageID && (evt.Type == MsgTypeText || evt.Type == MsgTypeReasoning || evt.Type == MsgTypeUsageReported) {
			continue
		}
		updated = append(updated, evt)
	}
	positions[len(ctx.msg.Events)] = len(updated)
	// 工具和并行消息保存的是事件下标，投影长度变化后同步映射到新位置。
	for i := range ctx.pendingToolCalls {
		tc := &ctx.pendingToolCalls[i]
		if tc.EventIndex >= 0 {
			tc.EventIndex = positions[tc.EventIndex]
		}
	}
	for otherKey, other := range ctx.pendingAssistant {
		if otherKey != key {
			other.firstEvent = positions[other.firstEvent]
			ctx.pendingAssistant[otherKey] = other
		}
	}
	ctx.msg.Events = updated
}

func (h *HistoryRecorder) finalizeActiveMessage(key string) {
	ctx := h.activeMessages[key]
	if ctx == nil {
		return
	}
	h.flushAssistantMessages(ctx)
	h.flushChunkedToolResults(ctx)
	if len(ctx.msg.Events) > 0 {
		ctx.msg.EndTime = time.Now()
		h.messages = append(h.messages, ctx.msg)
	}
	delete(h.activeMessages, key)
}

func (h *HistoryRecorder) finalizeAllActiveMessages() {
	for _, key := range h.sortedActiveKeysLocked() {
		h.finalizeActiveMessage(key)
	}
	h.activeOrder = nil
}

func (h *HistoryRecorder) sortedActiveKeysLocked() []string {
	keys := make([]string, 0, len(h.activeOrder))
	for _, key := range h.activeOrder {
		if h.activeMessages[key] != nil {
			keys = append(keys, key)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a := h.activeMessages[keys[i]]
		b := h.activeMessages[keys[j]]
		if a == nil || b == nil {
			return a != nil
		}
		if a.createdSeq != b.createdSeq {
			return a.createdSeq < b.createdSeq
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.msg.StartTime.Before(b.msg.StartTime)
	})
	return keys
}

// RecordEvent 记录事件
func (h *HistoryRecorder) RecordEvent(event Event) {
	event = events.NormalizeEvent(event)

	h.mu.Lock()
	defer h.mu.Unlock()

	if event.Type == events.EventUserMessage {
		h.finalizeAllActiveMessages()
	}
	if event.Type == EventError || event.Type == events.EventCancelled {
		if event.AgentName == "" && event.MemberCallID == "" && event.RunPath == "" {
			for _, key := range h.sortedActiveKeysLocked() {
				h.flushAssistantMessages(h.activeMessages[key])
			}
		} else {
			h.flushAssistantMessages(h.ensureMessageContext(event))
		}
	}
	if event.Type == events.EventAssistantCompleted && event.Role != message.RoleTool && event.Role != message.RoleUser {
		h.completeAssistantMessage(event)
	}
	h.recordTranscript(event)

	switch event.Type {
	case events.EventUserMessage:
		msg := message.Message{Role: message.RoleUser, Content: event.Content}
		if event.Message != nil {
			msg = *event.Message
			if msg.Role == "" {
				msg.Role = message.RoleUser
			}
		}
		if msg.Role != message.RoleUser || msg.IsEmpty() {
			return
		}
		h.finalizeAllActiveMessages()
		evt := historyEventEnvelope(event, MsgTypeText)
		evt.Content = msg.DisplayText()
		evt.ContentParts = append([]message.ContentPart(nil), msg.ContentParts...)
		createdAt := event.CreatedAt
		h.messages = append(h.messages, AgentMessage{
			AgentName: "user",
			StartTime: createdAt,
			EndTime:   createdAt,
			Events: []MessageEvent{
				evt,
			},
		})

	case EventUsageReported:
		usage := usageRecordFromEvent(event)
		if usage == nil {
			return
		}
		ctx := h.ensureMessageContext(event)
		evt := historyEventEnvelope(event, MsgTypeUsageReported)
		evt.Usage = usage
		ctx.msg.Events = append(ctx.msg.Events, evt)

	case events.EventAssistantReasoning, events.EventAssistantText:
		if event.Role == message.RoleUser {
			return
		}
		content := event.Content
		if content == "" {
			return
		}
		ctx := h.ensureMessageContext(event)
		deltaKind := event.DeltaKind
		if event.Type == events.EventAssistantReasoning {
			deltaKind = events.DeltaReasoning
		}
		if event.Type == events.EventAssistantText {
			deltaKind = events.DeltaOutput
		}
		if event.Role == message.RoleTool && (deltaKind == "" || deltaKind == events.DeltaOutput) {
			if key := toolResultKey(event); key != "" {
				ctx.toolResultChunks[key] += content
			}
			return
		}
		switch deltaKind {
		case events.DeltaReasoning:

			if n := len(ctx.msg.Events); n > 0 && ctx.msg.Events[n-1].Type == MsgTypeReasoning && ctx.msg.Events[n-1].RunID == event.RunID && ctx.msg.Events[n-1].MessageID == event.MessageID {
				ctx.msg.Events[n-1].Content += content
			} else {
				evt := historyEventEnvelope(event, MsgTypeReasoning)
				evt.Content = content
				ctx.msg.Events = append(ctx.msg.Events, evt)
			}
		case events.DeltaOutput, "":

			if n := len(ctx.msg.Events); n > 0 && ctx.msg.Events[n-1].Type == MsgTypeText && ctx.msg.Events[n-1].RunID == event.RunID && ctx.msg.Events[n-1].MessageID == event.MessageID {
				ctx.msg.Events[n-1].Content += content
			} else {
				evt := historyEventEnvelope(event, MsgTypeText)
				evt.Content = content
				ctx.msg.Events = append(ctx.msg.Events, evt)
			}
		case events.DeltaToolResult:
			if events.IsInternalContinueContent(content) {
				return
			}
			if key := toolResultKey(event); key != "" {
				ctx.toolResultChunks[key] += content
			}
		}

	case EventToolCallStarted:
		ctx := h.ensureMessageContext(event)
		toolCalls := events.ToolCallsFromEvent(event)
		for i, tc := range toolCalls {
			if events.IsInternalToolName(tc.Function.Name) {
				continue
			}
			if tc.Function.Name == "" {
				continue
			}
			ref := events.ToolCallRefAt(event, tc, i)
			if ref == "" {
				continue
			}
			updated := false
			for i := range ctx.pendingToolCalls {
				sameRef := ctx.pendingToolCalls[i].Ref != "" && ctx.pendingToolCalls[i].Ref == ref
				if sameRef {
					ctx.pendingToolCalls[i].Ref = ref
					if tc.ID != "" {
						ctx.pendingToolCalls[i].ID = tc.ID
					}
					ctx.pendingToolCalls[i].Arguments = tc.Function.Arguments
					if tc.Extra != nil {
						ctx.pendingToolCalls[i].Extra = message.CloneMetadata(tc.Extra)
					}
					h.updateToolCallEvent(ctx, ctx.pendingToolCalls[i], "")
					updated = true
					break
				}
			}
			if !updated {
				pending := h.pendingToolCallFromEvent(event, ref, tc.ID, tc.Index, tc.Function.Name, tc.Function.Arguments, tc.Extra)
				pending.EventIndex = h.appendToolCallEvent(ctx, pending)
				ctx.pendingToolCalls = append(ctx.pendingToolCalls, pending)
			}
		}

	case EventToolCallResult:
		content := event.Content
		if content == "" {
			content = event.ToolResult
		}
		if events.IsInternalContinueContent(content) {
			return
		}
		ctx := h.ensureMessageContext(event)
		if key := toolResultKey(event); key != "" {
			ctx.toolResultChunks[key] += content
		}

	case EventToolCallCompleted:
		content := toolResultContentFromEvent(event)
		if events.IsInternalContinueContent(content) {
			return
		}
		ctx := h.ensureMessageContext(event)
		resultKey := toolResultKey(event)
		if resultKey != "" && ctx.toolResultChunks[resultKey] != "" {
			chunked := ctx.toolResultChunks[resultKey]
			if content == "" || strings.Contains(chunked, content) {
				content = chunked
			} else {
				content = chunked + content
			}
			delete(ctx.toolResultChunks, resultKey)
		}
		h.recordToolResult(ctx, event, content)

	case events.EventAskRequested, events.EventAskAnswered:
		ctx := h.ensureMessageContext(event)
		record := &AskRecord{}
		if event.Ask != nil {
			record.ID = event.Ask.ID
			record.Question = event.Ask.Question
			record.Options = append([]string(nil), event.Ask.Options...)
			record.MultiSelect = event.Ask.MultiSelect
			record.Selected = append([]string(nil), event.Ask.Selected...)
			record.FreeText = event.Ask.FreeText
		}
		if record.ID == "" {
			record.ID = event.Detail
		}
		if record.Question == "" && event.Type == events.EventAskRequested {
			record.Question = event.Content
		}
		if event.Type == events.EventAskAnswered {
			record.Answered = true
			if record.FreeText == "" && len(record.Selected) == 0 {
				record.FreeText = event.Content
			}
		}
		evt := historyEventEnvelope(event, MsgTypeAsk)
		evt.Ask = record
		ctx.msg.Events = append(ctx.msg.Events, evt)

	case EventSystemNotice:
		ctx := h.ensureMessageContext(event)
		evt := historyEventEnvelope(event, MsgTypeNotice)
		evt.Content = event.Content
		evt.Detail = event.Detail
		ctx.msg.Events = append(ctx.msg.Events, evt)

	case EventError:
		ctx := h.ensureMessageContext(event)
		friendly := events.NormalizeFriendlyError(event.Error)
		friendly.TechnicalDetail = truncateErrorContent(friendly.TechnicalDetail)
		historyError := FriendlyError(friendly)
		evt := historyEventEnvelope(event, MsgTypeError)
		evt.Content = historyError.Message
		evt.Error = &historyError
		ctx.msg.Events = append(ctx.msg.Events, evt)

	}
}

func (h *HistoryRecorder) flushChunkedToolResults(ctx *activeMessageContext) {
	if ctx == nil || len(ctx.toolResultChunks) == 0 {
		return
	}
	for resultKey, content := range ctx.toolResultChunks {
		if content == "" {
			delete(ctx.toolResultChunks, resultKey)
			continue
		}
		idx := -1
		for i := range ctx.pendingToolCalls {
			if pendingToolCallMatchesResultKey(ctx.pendingToolCalls[i], resultKey) {
				idx = i
				break
			}
		}
		if idx < 0 {
			delete(ctx.toolResultChunks, resultKey)
			continue
		}
		tc := ctx.pendingToolCalls[idx]
		ctx.pendingToolCalls = append(ctx.pendingToolCalls[:idx], ctx.pendingToolCalls[idx+1:]...)
		if !events.IsInternalToolName(tc.Name) {
			if !h.updateToolCallEvent(ctx, tc, content) {
				ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
					Type:      MsgTypeToolCall,
					EventID:   tc.EventID,
					Sequence:  tc.Sequence,
					CreatedAt: tc.CreatedAt,
					RunID:     tc.RunID,
					TurnID:    tc.TurnID,
					MessageID: tc.MessageID,
					ToolCall:  ptrToolCallRecord(toolCallRecordFromPending(tc, content)),
				})
			}
		}
		delete(ctx.toolResultChunks, resultKey)
	}
}

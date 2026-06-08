package eventlog

import (
	"fkteams/agentcore"

	"fkteams/events"

	"sort"
	"strings"

	"time"
)

func NewHistoryRecorder() *HistoryRecorder {
	return &HistoryRecorder{
		activeMessages: make(map[string]*activeMessageContext),
		activeOrder:    make([]string, 0),
		messages:       make([]AgentMessage, 0),
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
	return tc.Ref != "" && tc.Ref == event.ToolCallRef
}

func eventMatchesToolCallRecord(record *ToolCallRecord, event Event) bool {
	if record == nil {
		return false
	}
	return record.Ref != "" && record.Ref == event.ToolCallRef
}

func (h *HistoryRecorder) recordToolResult(ctx *activeMessageContext, event Event, content string) {
	if ctx == nil || content == "" || events.IsInternalContinueContent(content) {
		return
	}
	if event.ToolCallRef == "" {
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
				Type:     MsgTypeToolCall,
				ToolCall: ptrToolCallRecord(toolCallRecordFromPending(tc, content)),
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
	if event.ToolName == "" || event.ToolCallRef == "" {
		return
	}
	pending := pendingToolCallFromEvent(event.ToolCallRef, event.ToolCallID, event.ToolCallIndex, event.ToolName, event.ToolArgs)
	if !events.IsInternalToolName(pending.Name) {
		ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
			Type:     MsgTypeToolCall,
			ToolCall: ptrToolCallRecord(toolCallRecordFromPending(pending, content)),
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

func (h *HistoryRecorder) finalizeActiveMessage(key string) {
	ctx := h.activeMessages[key]
	if ctx == nil {
		return
	}
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

	switch event.Type {
	case EventUsage:
		if event.PromptTokens == 0 && event.CompletionTokens == 0 && event.TotalTokens == 0 {
			return
		}
		ctx := h.ensureMessageContext(event)
		ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
			Type: MsgTypeUsage,
			Usage: &UsageRecord{
				PromptTokens:     event.PromptTokens,
				CompletionTokens: event.CompletionTokens,
				TotalTokens:      event.TotalTokens,
			},
		})

	case EventMessageDelta:
		if event.Role == agentcore.RoleUser {
			return
		}
		content := event.Content
		if content == "" {
			return
		}
		ctx := h.ensureMessageContext(event)
		if event.Role == agentcore.RoleTool && (event.DeltaKind == "" || event.DeltaKind == events.DeltaOutput) {
			if key := toolResultKey(event); key != "" {
				ctx.toolResultChunks[key] += content
			}
			return
		}
		switch event.DeltaKind {
		case events.DeltaReasoning:

			if n := len(ctx.msg.Events); n > 0 && ctx.msg.Events[n-1].Type == MsgTypeReasoning {
				ctx.msg.Events[n-1].Content += content
			} else {
				ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
					Type:    MsgTypeReasoning,
					Content: content,
				})
			}
		case events.DeltaOutput, "":

			if n := len(ctx.msg.Events); n > 0 && ctx.msg.Events[n-1].Type == MsgTypeText {
				ctx.msg.Events[n-1].Content += content
			} else {
				ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
					Type:    MsgTypeText,
					Content: content,
				})
			}
		case events.DeltaToolResult:
			if events.IsInternalContinueContent(content) {
				return
			}
			if key := toolResultKey(event); key != "" {
				ctx.toolResultChunks[key] += content
			}
		}

	case events.EventMessageEnd:
		if event.Role != agentcore.RoleTool {
			return
		}
		content := toolResultContentFromEvent(event)
		if content == "" || events.IsInternalContinueContent(content) {
			return
		}
		ctx := h.ensureMessageContext(event)
		resultKey := toolResultKey(event)
		if resultKey != "" && ctx.toolResultChunks[resultKey] != "" {
			chunked := ctx.toolResultChunks[resultKey]
			if strings.Contains(chunked, content) {
				content = chunked
			} else if !strings.Contains(content, chunked) {
				content = chunked + content
			}
			delete(ctx.toolResultChunks, resultKey)
		}
		h.recordToolResult(ctx, event, content)

	case EventToolStart:
		ctx := h.ensureMessageContext(event)
		toolCalls := event.ToolCalls
		if event.ToolCall != nil {
			toolCalls = append([]agentcore.ToolCall{*event.ToolCall}, toolCalls...)
		}
		if len(toolCalls) == 0 && event.ToolName != "" {
			toolCalls = []agentcore.ToolCall{{
				ID:    event.ToolCallID,
				Index: event.ToolCallIndex,
				Function: agentcore.FunctionCall{
					Name:      event.ToolName,
					Arguments: event.ToolArgs,
				},
			}}
		}
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
					h.updateToolCallEvent(ctx, ctx.pendingToolCalls[i], "")
					updated = true
					break
				}
			}
			if !updated {
				pending := pendingToolCallFromEvent(ref, tc.ID, tc.Index, tc.Function.Name, tc.Function.Arguments)
				pending.EventIndex = h.appendToolCallEvent(ctx, pending)
				ctx.pendingToolCalls = append(ctx.pendingToolCalls, pending)
			}
		}

	case EventToolUpdate:
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

	case EventToolEnd:
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

	case EventAction:
		ctx := h.ensureMessageContext(event)
		ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
			Type: MsgTypeAction,
			Action: &ActionRecord{
				ActionType: event.ActionType,
				Content:    event.Content,
				Detail:     event.Detail,
			},
		})

	case EventError:
		ctx := h.ensureMessageContext(event)
		ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
			Type:    MsgTypeError,
			Content: truncateErrorContent(event.Error),
		})

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
			sameRef := ctx.pendingToolCalls[i].Ref != "" && ctx.pendingToolCalls[i].Ref == resultKey
			if sameRef {
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
					Type:     MsgTypeToolCall,
					ToolCall: ptrToolCallRecord(toolCallRecordFromPending(tc, content)),
				})
			}
		}
		delete(ctx.toolResultChunks, resultKey)
	}
}

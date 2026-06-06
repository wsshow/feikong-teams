package eventlog

import (
	"encoding/json"
	"fkteams/agentcore"
	"fkteams/agents/toolmeta"
	"fkteams/common"
	"fkteams/common/atomicfile"
	"fkteams/events"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Event = events.Event
type ActionType = events.ActionType

const (
	EventMessageDelta = events.EventMessageDelta
	EventToolStart    = events.EventToolStart
	EventToolUpdate   = events.EventToolUpdate
	EventToolEnd      = events.EventToolEnd
	EventAction       = events.EventAction
	EventUsage        = events.EventUsage
	EventError        = events.EventError

	ActionContextCompress = events.ActionContextCompress
)

const HistoryFileName = "history.jsonl"

type ToolCallRecord struct {
	SpanID      string `json:"span_id,omitempty"`
	Ref         string `json:"ref,omitempty"`
	ID          string `json:"id"`
	Index       *int   `json:"index,omitempty"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Target      string `json:"target,omitempty"`
	Arguments   string `json:"arguments"`
	Result      string `json:"result"`
}

type ActionRecord struct {
	ActionType ActionType `json:"action_type"`
	Content    string     `json:"content"`
	Detail     string     `json:"detail,omitempty"`
}

type UsageRecord struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

const historyLineTypeMessageEvent = "message_event"

type HistoryLine struct {
	Type           string       `json:"type"`
	MessageID      string       `json:"message_id"`
	EventIndex     int          `json:"event_index"`
	SpanID         string       `json:"span_id,omitempty"`
	ParentSpanID   string       `json:"parent_span_id,omitempty"`
	AgentName      string       `json:"agent_name"`
	RunPath        string       `json:"run_path,omitempty"`
	MemberCallID   string       `json:"member_call_id,omitempty"`
	MemberToolName string       `json:"member_tool_name,omitempty"`
	MemberName     string       `json:"member_name,omitempty"`
	IsMemberEvent  bool         `json:"is_member_event,omitempty"`
	StartTime      time.Time    `json:"start_time"`
	EndTime        time.Time    `json:"end_time"`
	Event          MessageEvent `json:"event"`
}

// MsgEventType 历史消息事件类型
type MsgEventType string

const (
	MsgTypeText      MsgEventType = "text"
	MsgTypeReasoning MsgEventType = "reasoning"
	MsgTypeToolCall  MsgEventType = "tool_call"
	MsgTypeAction    MsgEventType = "action"
	MsgTypeUsage     MsgEventType = "usage"
	MsgTypeError     MsgEventType = "error"
	MsgTypeCancelled MsgEventType = "cancelled"
)

// MessageEvent 单个消息事件
type MessageEvent struct {
	Type         MsgEventType            `json:"type"`
	Content      string                  `json:"content,omitempty"`
	ContentParts []agentcore.ContentPart `json:"content_parts,omitempty"`
	ToolCall     *ToolCallRecord         `json:"tool_call,omitempty"`
	Action       *ActionRecord           `json:"action,omitempty"`
	Usage        *UsageRecord            `json:"usage,omitempty"`
}

// AgentMessage 代理的一次完整发言
type AgentMessage struct {
	SpanID         string         `json:"span_id,omitempty"`
	ParentSpanID   string         `json:"parent_span_id,omitempty"`
	AgentName      string         `json:"agent_name"`
	RunPath        string         `json:"run_path"`
	MemberCallID   string         `json:"member_call_id,omitempty"`
	MemberToolName string         `json:"member_tool_name,omitempty"`
	MemberName     string         `json:"member_name,omitempty"`
	IsMemberEvent  bool           `json:"is_member_event,omitempty"`
	StartTime      time.Time      `json:"start_time"`
	EndTime        time.Time      `json:"end_time"`
	Events         []MessageEvent `json:"events"`
}

// GetTextContent 获取消息中的所有文本内容
func (m *AgentMessage) GetTextContent() string {
	var builder strings.Builder
	for _, event := range m.Events {
		if event.Type == MsgTypeText {
			builder.WriteString(event.Content)
		}
	}
	return builder.String()
}

// NoisyToolPrefixes 定义高输出量噪声工具的名称前缀列表。
// 这类工具（如网页抓取、文档读取）会产生大量输出，在历史上下文中属于冗余内容。
var NoisyToolPrefixes = []string{"fetch", "doc"}

// GetReasoningContent 获取消息中的推理/思考内容
func (m *AgentMessage) GetReasoningContent() string {
	var builder strings.Builder
	for _, event := range m.Events {
		if event.Type == MsgTypeReasoning {
			builder.WriteString(event.Content)
		}
	}
	return builder.String()
}

// 错误内容最大长度（rune），超出时保留头尾并截断中间部分
const maxErrorContentLen = 1200

// pendingToolCall 待匹配的工具调用
type pendingToolCall struct {
	SpanID      string
	Ref         string
	ID          string
	Index       *int
	EventIndex  int
	Name        string
	DisplayName string
	Kind        string
	Target      string
	Arguments   string
}

type activeMessageContext struct {
	msg              AgentMessage
	pendingToolCalls []pendingToolCall
	toolResultChunks map[string]string
	order            int
	createdSeq       int64
}

// HistoryRecorder 事件历史记录器
type HistoryRecorder struct {
	mu                sync.RWMutex
	messages          []AgentMessage
	activeMessages    map[string]*activeMessageContext
	activeOrder       []string
	currentAgent      string
	currentRunPath    string
	currentMemberID   string
	currentMemberTool string
	currentMemberName string
	currentEvents     []MessageEvent
	pendingToolCalls  []pendingToolCall // 按 ID 匹配工具调用与结果
	toolResultChunks  map[string]string // 按 ID 累积流式工具结果
	summary           string            // 上下文压缩摘要
	summarizedCount   int               // 已被摘要覆盖的消息数量
}

func toolCallRecordFromPending(tc pendingToolCall, result string) ToolCallRecord {
	record := ToolCallRecord{
		SpanID:      tc.SpanID,
		Ref:         tc.Ref,
		ID:          tc.ID,
		Index:       tc.Index,
		Name:        tc.Name,
		DisplayName: tc.DisplayName,
		Kind:        tc.Kind,
		Target:      tc.Target,
		Arguments:   tc.Arguments,
		Result:      result,
	}
	if record.DisplayName == "" || record.Kind == "" {
		display := toolmeta.FormatToolDisplay(tc.Name)
		if record.DisplayName == "" {
			record.DisplayName = display.DisplayName
		}
		if record.Kind == "" {
			record.Kind = display.Kind
		}
		if record.Target == "" {
			record.Target = display.Target
		}
	}
	return record
}

func ptrToolCallRecord(record ToolCallRecord) *ToolCallRecord {
	return &record
}

func pendingToolCallFromEvent(spanID, ref, id string, index *int, name, arguments string) pendingToolCall {
	display := toolmeta.FormatToolDisplay(name)
	return pendingToolCall{
		SpanID:      spanID,
		Ref:         ref,
		ID:          id,
		Index:       index,
		EventIndex:  -1,
		Name:        name,
		DisplayName: display.DisplayName,
		Kind:        display.Kind,
		Target:      display.Target,
		Arguments:   arguments,
	}
}

func toolCallSpanFromEvent(event Event, tc agentcore.ToolCall) string {
	if tc.Index != nil && event.ToolCallSpanIDs != nil {
		if span := event.ToolCallSpanIDs[*tc.Index]; span != "" {
			return span
		}
	}
	return event.SpanID
}

func (h *HistoryRecorder) appendToolCallEvent(ctx *activeMessageContext, tc pendingToolCall) int {
	record := toolCallRecordFromPending(tc, "")
	ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
		Type:     MsgTypeToolCall,
		ToolCall: ptrToolCallRecord(record),
	})
	return len(ctx.msg.Events) - 1
}

func (h *HistoryRecorder) updateToolCallEvent(ctx *activeMessageContext, tc pendingToolCall, result string) bool {
	if tc.EventIndex < 0 || tc.EventIndex >= len(ctx.msg.Events) {
		return false
	}
	if ctx.msg.Events[tc.EventIndex].Type != MsgTypeToolCall || ctx.msg.Events[tc.EventIndex].ToolCall == nil {
		return false
	}
	record := toolCallRecordFromPending(tc, result)
	ctx.msg.Events[tc.EventIndex].ToolCall = ptrToolCallRecord(record)
	return true
}

func NewHistoryRecorder() *HistoryRecorder {
	return &HistoryRecorder{
		activeMessages:   make(map[string]*activeMessageContext),
		activeOrder:      make([]string, 0),
		messages:         make([]AgentMessage, 0),
		currentEvents:    make([]MessageEvent, 0),
		toolResultChunks: make(map[string]string),
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
		content = event.Delta
	}
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
	pending := pendingToolCallFromEvent(event.SpanID, event.ToolCallRef, event.ToolCallID, event.ToolCallIndex, event.ToolName, event.ToolArgs)
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
			SpanID:         event.SpanID,
			ParentSpanID:   event.ParentSpanID,
			AgentName:      event.AgentName,
			RunPath:        event.RunPath,
			MemberCallID:   event.MemberCallID,
			MemberToolName: event.MemberToolName,
			MemberName:     event.MemberName,
			IsMemberEvent:  event.MemberCallID != "",
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
		content := event.Delta
		if content == "" {
			content = event.Content
		}
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
			// 合并连续推理事件
			if n := len(ctx.msg.Events); n > 0 && ctx.msg.Events[n-1].Type == MsgTypeReasoning {
				ctx.msg.Events[n-1].Content += content
			} else {
				ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
					Type:    MsgTypeReasoning,
					Content: content,
				})
			}
		case events.DeltaOutput, "":
			// 合并连续文本事件
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
				pending := pendingToolCallFromEvent(toolCallSpanFromEvent(event, tc), ref, tc.ID, tc.Index, tc.Function.Name, tc.Function.Arguments)
				pending.EventIndex = h.appendToolCallEvent(ctx, pending)
				ctx.pendingToolCalls = append(ctx.pendingToolCalls, pending)
			}
		}

	case EventToolUpdate:
		content := event.Delta
		if content == "" {
			content = event.Content
		}
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

func (h *HistoryRecorder) RecordUserMessage(message agentcore.Message) {
	if message.Role == "" {
		message.Role = agentcore.RoleUser
	}
	if message.Role != agentcore.RoleUser || message.IsEmpty() {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.finalizeAllActiveMessages()

	content := message.DisplayText()
	parts := append([]agentcore.ContentPart(nil), message.UserInputMultiContent...)
	if len(parts) == 0 {
		parts = append(parts, message.MultiContent...)
	}

	h.messages = append(h.messages, AgentMessage{
		AgentName: "用户",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Events: []MessageEvent{
			{Type: MsgTypeText, Content: content, ContentParts: parts},
		},
	})

	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
}

// RecordCancelled 记录用户取消任务事件，并标记当前仍活跃的消息。
func (h *HistoryRecorder) RecordCancelled(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if strings.TrimSpace(message) == "" {
		message = "任务已取消"
	}
	cancelEvent := MessageEvent{Type: MsgTypeCancelled, Content: message}
	for _, key := range h.sortedActiveKeysLocked() {
		ctx := h.activeMessages[key]
		if ctx == nil || messageHasEventType(ctx.msg.Events, MsgTypeCancelled) {
			continue
		}
		ctx.msg.Events = append(ctx.msg.Events, cancelEvent)
	}
	h.finalizeAllActiveMessages()
	if len(h.messages) > 0 && h.messages[len(h.messages)-1].AgentName == "系统" && messageHasEventType(h.messages[len(h.messages)-1].Events, MsgTypeCancelled) {
		return
	}
	h.messages = append(h.messages, AgentMessage{
		AgentName: "系统",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Events: []MessageEvent{
			cancelEvent,
		},
	})
}

func messageHasEventType(events []MessageEvent, typ MsgEventType) bool {
	for _, event := range events {
		if event.Type == typ {
			return true
		}
	}
	return false
}

// FinalizeCurrent 完成当前消息记录，在对话结束时调用
func (h *HistoryRecorder) FinalizeCurrent() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finalizeAllActiveMessages()
	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
	h.currentEvents = make([]MessageEvent, 0)
	h.pendingToolCalls = nil
	h.toolResultChunks = make(map[string]string)
	h.activeMessages = make(map[string]*activeMessageContext)
	h.activeOrder = nil
}

func (h *HistoryRecorder) GetMessages() []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]AgentMessage, len(h.messages))
	copy(result, h.messages)
	return result
}

func (h *HistoryRecorder) GetAgentMessages(agentName string) []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]AgentMessage, 0)
	for _, msg := range h.messages {
		if msg.AgentName == agentName {
			result = append(result, msg)
		}
	}
	return result
}

// GetCurrentMessage 返回当前构建中的 (agentName, textContent)
func (h *HistoryRecorder) GetCurrentMessage() (agentName, content string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var builder strings.Builder
	for _, key := range h.sortedActiveKeysLocked() {
		ctx := h.activeMessages[key]
		if ctx == nil {
			continue
		}
		if agentName == "" {
			agentName = ctx.msg.AgentName
		}
		for _, event := range ctx.msg.Events {
			if event.Type == MsgTypeText {
				builder.WriteString(event.Content)
			}
		}
	}
	if builder.Len() > 0 {
		return agentName, builder.String()
	}
	for _, event := range h.currentEvents {
		if event.Type == MsgTypeText {
			builder.WriteString(event.Content)
		}
	}
	return h.currentAgent, builder.String()
}

func (h *HistoryRecorder) GetFullHistory() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	messages := h.snapshotMessagesLocked()
	var result strings.Builder
	for i, msg := range messages {
		if i > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("=== ")
		result.WriteString(msg.AgentName)
		result.WriteString(" ===\n")
		for _, event := range msg.Events {
			if event.Type == MsgTypeText {
				result.WriteString(event.Content)
			}
		}
	}

	return result.String()
}

func (h *HistoryRecorder) GetConversationSummary() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result strings.Builder
	for i, msg := range h.messages {
		duration := msg.EndTime.Sub(msg.StartTime)
		var contentLen int
		for _, event := range msg.Events {
			if event.Type == MsgTypeText {
				contentLen += len([]rune(event.Content))
			}
		}
		fmt.Fprintf(&result, "%d. [%s] %s - %d字 (%v)\n",
			i+1, msg.StartTime.Format("15:04:05"), msg.AgentName, contentLen, duration.Round(time.Millisecond))
	}
	return result.String()
}

func (h *HistoryRecorder) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = make([]AgentMessage, 0)
	h.currentEvents = make([]MessageEvent, 0)
	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
	h.pendingToolCalls = nil
	h.toolResultChunks = make(map[string]string)
	h.activeMessages = make(map[string]*activeMessageContext)
	h.activeOrder = nil
	h.summary = ""
	h.summarizedCount = 0
}

// SetSummary 设置上下文压缩摘要
func (h *HistoryRecorder) SetSummary(text string, count int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.summary = text
	h.summarizedCount = count
}

// GetSummary 获取上下文压缩摘要和已覆盖的消息数量
func (h *HistoryRecorder) GetSummary() (string, int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.summary, h.summarizedCount
}

// reconstructSummaryFromEvents 从事件流中重建上下文压缩摘要状态（需在持锁状态下调用）
func (h *HistoryRecorder) reconstructSummaryFromEvents() {
	h.summary = ""
	h.summarizedCount = 0

	// 从后向前查找最后一个 context_compress 事件
	for i := len(h.messages) - 1; i >= 0; i-- {
		for _, evt := range h.messages[i].Events {
			if evt.Type == MsgTypeAction && evt.Action != nil &&
				evt.Action.ActionType == ActionContextCompress && evt.Action.Detail != "" {
				h.summary = evt.Action.Detail
				// 向前查找该事件所属执行轮次的用户输入
				for j := i - 1; j >= 0; j-- {
					if h.messages[j].AgentName == "用户" {
						h.summarizedCount = j
						return
					}
				}
				h.summarizedCount = 0
				return
			}
		}
	}
}
func (h *HistoryRecorder) GetAgentNames() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nameMap := make(map[string]bool)
	for _, msg := range h.messages {
		nameMap[msg.AgentName] = true
	}
	for _, ctx := range h.activeMessages {
		if ctx != nil && ctx.msg.AgentName != "" {
			nameMap[ctx.msg.AgentName] = true
		}
	}

	names := make([]string, 0, len(nameMap))
	for name := range nameMap {
		names = append(names, name)
	}
	return names
}

func (h *HistoryRecorder) GetMessageCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.snapshotMessagesLocked())
}

func (h *HistoryRecorder) SaveToFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return saveMessagesToFile(h.snapshotMessagesLocked(), filePath)
}

func (h *HistoryRecorder) snapshotMessagesLocked() []AgentMessage {
	messages := make([]AgentMessage, len(h.messages))
	copy(messages, h.messages)
	for _, key := range h.activeOrder {
		ctx := h.activeMessages[key]
		if ctx == nil || len(ctx.msg.Events) == 0 {
			continue
		}
		msg := ctx.msg
		msg.EndTime = time.Now()
		messages = append(messages, msg)
	}
	return messages
}

func saveMessagesToFile(messages []AgentMessage, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	data, err := marshalMessagesJSONL(messages)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func marshalMessagesJSONL(messages []AgentMessage) ([]byte, error) {
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	for msgIndex, msg := range messages {
		messageID := historyMessageID(msg, msgIndex)
		for eventIndex, event := range msg.Events {
			line := HistoryLine{
				Type:           historyLineTypeMessageEvent,
				MessageID:      messageID,
				EventIndex:     eventIndex,
				SpanID:         msg.SpanID,
				ParentSpanID:   msg.ParentSpanID,
				AgentName:      msg.AgentName,
				RunPath:        msg.RunPath,
				MemberCallID:   msg.MemberCallID,
				MemberToolName: msg.MemberToolName,
				MemberName:     msg.MemberName,
				IsMemberEvent:  msg.IsMemberEvent,
				StartTime:      msg.StartTime,
				EndTime:        msg.EndTime,
				Event:          event,
			}
			if err := encoder.Encode(line); err != nil {
				return nil, fmt.Errorf("marshal jsonl: %w", err)
			}
		}
	}
	return []byte(builder.String()), nil
}

func historyMessageID(msg AgentMessage, index int) string {
	return fmt.Sprintf("%06d:%s:%s", index, msg.AgentName, msg.StartTime.UTC().Format(time.RFC3339Nano))
}

func (h *HistoryRecorder) LoadFromFile(filePath string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	defer file.Close()

	messages, err := loadMessagesJSONL(file)
	if err != nil {
		return err
	}
	h.messages = messages

	// 从事件流中重建上下文压缩摘要状态
	h.reconstructSummaryFromEvents()

	// 替换当前数据
	h.currentAgent = ""
	h.currentEvents = make([]MessageEvent, 0)
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
	h.pendingToolCalls = nil
	h.toolResultChunks = make(map[string]string)
	h.activeMessages = make(map[string]*activeMessageContext)
	h.activeOrder = nil

	return nil
}

func loadMessagesJSONL(file *os.File) ([]AgentMessage, error) {
	messages := make([]AgentMessage, 0)
	messageIndex := make(map[string]int)
	decoder := json.NewDecoder(file)
	lineNo := 1
	for {
		var line HistoryLine
		if err := decoder.Decode(&line); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode jsonl record %d: %w", lineNo, err)
		}
		if line.Type != historyLineTypeMessageEvent {
			return nil, fmt.Errorf("unsupported history line type at record %d: %s", lineNo, line.Type)
		}
		if line.MessageID == "" {
			return nil, fmt.Errorf("missing message_id at record %d", lineNo)
		}
		idx, exists := messageIndex[line.MessageID]
		if !exists {
			messageIndex[line.MessageID] = len(messages)
			messages = append(messages, AgentMessage{
				SpanID:         line.SpanID,
				ParentSpanID:   line.ParentSpanID,
				AgentName:      line.AgentName,
				RunPath:        line.RunPath,
				MemberCallID:   line.MemberCallID,
				MemberToolName: line.MemberToolName,
				MemberName:     line.MemberName,
				IsMemberEvent:  line.IsMemberEvent,
				StartTime:      line.StartTime,
				EndTime:        line.EndTime,
				Events:         make([]MessageEvent, 0),
			})
			idx = len(messages) - 1
		}
		messages[idx].Events = append(messages[idx].Events, line.Event)
		messages[idx].EndTime = line.EndTime
		lineNo++
	}
	return messages, nil
}

func (h *HistoryRecorder) SaveToMarkdownFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return saveMessagesToMarkdown(h.snapshotMessagesLocked(), filePath)
}

func saveMessagesToMarkdown(messages []AgentMessage, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	var md strings.Builder

	md.WriteString("# 对话历史\n\n")
	fmt.Fprintf(&md, "**生成时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&md, "**对话轮次**: %d\n\n", len(messages))

	agentMap := make(map[string]int)
	for _, msg := range messages {
		agentMap[msg.AgentName]++
	}
	md.WriteString("**参与代理**: ")
	first := true
	for agent, count := range agentMap {
		if !first {
			md.WriteString(", ")
		}
		fmt.Fprintf(&md, "%s (%d次)", agent, count)
		first = false
	}
	md.WriteString("\n\n---\n\n")

	// 对话内容
	for i, msg := range messages {
		fmt.Fprintf(&md, "## %d. %s\n\n", i+1, msg.AgentName)

		duration := msg.EndTime.Sub(msg.StartTime)
		fmt.Fprintf(&md, "**时间**: %s - %s (%v)\n\n",
			msg.StartTime.Format("15:04:05"),
			msg.EndTime.Format("15:04:05"),
			duration.Round(time.Millisecond))

		if msg.RunPath != "" {
			fmt.Fprintf(&md, "**路径**: `%s`\n\n", msg.RunPath)
		}

		// 事件内容
		md.WriteString("**内容**:\n\n")
		for _, event := range msg.Events {
			switch event.Type {
			case MsgTypeText:
				md.WriteString(event.Content)
				md.WriteString("\n\n")

			case MsgTypeToolCall:
				if event.ToolCall != nil {
					display := toolmeta.FormatToolDisplay(event.ToolCall.Name)
					fmt.Fprintf(&md, "> **工具调用**: %s\n", display.DisplayName)
					if event.ToolCall.Arguments != "" {
						fmt.Fprintf(&md, "> - **参数**: `%s`\n", event.ToolCall.Arguments)
					}
					if event.ToolCall.Result != "" {
						fmt.Fprintf(&md, "> - **结果**: %s\n", event.ToolCall.Result)
					}
					md.WriteString("\n")
				}

			case MsgTypeAction:
				if event.Action != nil && (event.Action.ActionType != "" || event.Action.Content != "") {
					fmt.Fprintf(&md, "> **[Action]**: [%s] %s\n\n", event.Action.ActionType, event.Action.Content)
				}

			case MsgTypeError:
				fmt.Fprintf(&md, "> **[错误]**: %s\n\n", event.Content)
			}
		}

		// 分隔线（除了最后一条消息）
		if i < len(messages)-1 {
			md.WriteString("---\n\n")
		}
	}

	if err := atomicfile.WriteFile(filePath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (h *HistoryRecorder) SaveToMarkdownWithTimestamp() (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(common.AppDir(), "history", "output_history", fmt.Sprintf("chat_%s.md", timestamp))
	err := h.SaveToMarkdownFile(filePath)
	return filePath, err
}

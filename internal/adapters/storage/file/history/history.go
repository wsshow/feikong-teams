package eventlog

import (
	"fkteams/agents/toolmeta"
	domainhistory "fkteams/internal/domain/history"

	"fkteams/internal/runtime/events"

	"sync"
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

type ToolCallRecord = domainhistory.ToolCallRecord
type ActionRecord = domainhistory.ActionRecord
type UsageRecord = domainhistory.UsageRecord
type FriendlyError = domainhistory.FriendlyError
type HistoryLine = domainhistory.Line
type MsgEventType = domainhistory.MsgEventType
type MessageEvent = domainhistory.MessageEvent
type AgentMessage = domainhistory.AgentMessage
type AttachmentRef = domainhistory.AttachmentRef

const historyLineTypeMessageEvent = "message_event"

const (
	MsgTypeText      = domainhistory.MsgTypeText
	MsgTypeReasoning = domainhistory.MsgTypeReasoning
	MsgTypeToolCall  = domainhistory.MsgTypeToolCall
	MsgTypeAction    = domainhistory.MsgTypeAction
	MsgTypeUsage     = domainhistory.MsgTypeUsage
	MsgTypeError     = domainhistory.MsgTypeError
	MsgTypeCancelled = domainhistory.MsgTypeCancelled
)

// NoisyToolPrefixes 定义高输出量噪声工具的名称前缀列表。
// 这类工具（如网页抓取、文档读取）会产生大量输出，在历史上下文中属于冗余内容。
var NoisyToolPrefixes = []string{"fetch", "doc"}

// 错误内容最大长度（rune），超出时保留头尾并截断中间部分
const maxErrorContentLen = 1200

// pendingToolCall 待匹配的工具调用
type pendingToolCall struct {
	Ref         string
	ID          string
	Index       *int
	EventIndex  int
	Sequence    int64
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
	mu              sync.RWMutex
	messages        []AgentMessage
	activeMessages  map[string]*activeMessageContext
	activeOrder     []string
	summary         string // 上下文压缩摘要
	summarizedCount int    // 已被摘要覆盖的消息数量
}

func toolCallRecordFromPending(tc pendingToolCall, result string) ToolCallRecord {
	record := ToolCallRecord{
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

func pendingToolCallFromEvent(ref, id string, index *int, name, arguments string, sequence int64) pendingToolCall {
	display := toolmeta.FormatToolDisplay(name)
	return pendingToolCall{
		Ref:         ref,
		ID:          id,
		Index:       index,
		EventIndex:  -1,
		Sequence:    sequence,
		Name:        name,
		DisplayName: display.DisplayName,
		Kind:        display.Kind,
		Target:      display.Target,
		Arguments:   arguments,
	}
}

func (h *HistoryRecorder) appendToolCallEvent(ctx *activeMessageContext, tc pendingToolCall, sequence int64) int {
	record := toolCallRecordFromPending(tc, "")
	ctx.msg.Events = append(ctx.msg.Events, MessageEvent{
		Sequence: sequence,
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

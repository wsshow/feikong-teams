package eventlog

import (
	"fkteams/agentcore"
	"fkteams/agents/toolmeta"

	"fkteams/events"

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
	AgentName      string       `json:"agent_name"`
	RunPath        string       `json:"run_path,omitempty"`
	MemberCallID   string       `json:"member_call_id,omitempty"`
	MemberToolName string       `json:"member_tool_name,omitempty"`
	MemberName     string       `json:"member_name,omitempty"`
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
	Sequence     int64                   `json:"sequence,omitempty"`
	Content      string                  `json:"content,omitempty"`
	ContentParts []agentcore.ContentPart `json:"content_parts,omitempty"`
	Error        *events.FriendlyError   `json:"error,omitempty"`
	ToolCall     *ToolCallRecord         `json:"tool_call,omitempty"`
	Action       *ActionRecord           `json:"action,omitempty"`
	Usage        *UsageRecord            `json:"usage,omitempty"`
}

// AgentMessage 代理的一次完整发言
type AgentMessage struct {
	AgentName      string         `json:"agent_name"`
	RunPath        string         `json:"run_path"`
	MemberCallID   string         `json:"member_call_id,omitempty"`
	MemberToolName string         `json:"member_tool_name,omitempty"`
	MemberName     string         `json:"member_name,omitempty"`
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

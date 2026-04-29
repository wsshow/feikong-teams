package fkevent

import "github.com/cloudwego/eino/schema"

// EventType 事件类型
type EventType string

const (
	EventError              EventType = "error"
	EventMessage            EventType = "message"
	EventToolResult         EventType = "tool_result"
	EventReasoningChunk     EventType = "reasoning_chunk"
	EventStreamChunk        EventType = "stream_chunk"
	EventToolResultChunk    EventType = "tool_result_chunk"
	EventToolCallsPreparing EventType = "tool_calls_preparing"
	EventToolCallsArgsDelta EventType = "tool_calls_args_delta"
	EventToolCalls          EventType = "tool_calls"
	EventAction             EventType = "action"
	EventDispatchProgress   EventType = "dispatch_progress"
)

// ActionType 动作类型（EventAction 事件下的子类型）
type ActionType string

const (
	ActionTransfer             ActionType = "transfer"
	ActionInterrupted          ActionType = "interrupted"
	ActionExit                 ActionType = "exit"
	ActionAskQuestions         ActionType = "ask_questions"
	ActionAskResponse          ActionType = "ask_response"
	ActionApprovalRequired     ActionType = "approval_required"
	ActionApprovalDecision     ActionType = "approval_decision"
	ActionContextCompressStart ActionType = "context_compress_start"
	ActionContextCompress      ActionType = "context_compress"
)

// NotifyType SSE/WebSocket 通知消息类型
type NotifyType string

const (
	NotifyProcessingStart  NotifyType = "processing_start"
	NotifyProcessingEnd    NotifyType = "processing_end"
	NotifyCancelled        NotifyType = "cancelled"
	NotifyError            NotifyType = "error"
	NotifyAskQuestions     NotifyType = "ask_questions"
	NotifyApprovalRequired NotifyType = "approval_required"
	NotifyConnected        NotifyType = "connected"
	NotifyPong             NotifyType = "pong"
	NotifyInvalidAPIKey    NotifyType = "invalid_api_key"
)

// Event 统一的事件结构，承载各类智能体输出
type Event struct {
	Type             EventType         `json:"type"`
	AgentName        string            `json:"agent_name,omitempty"`
	RunPath          string            `json:"run_path,omitempty"`
	Content          string            `json:"content,omitempty"`
	Detail           string            `json:"detail,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"` // 推理模型思考内容
	ToolCalls        []schema.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"` // 工具结果对应的调用 ID
	ActionType       ActionType        `json:"action_type,omitempty"`
	Error            string            `json:"error,omitempty"`
}

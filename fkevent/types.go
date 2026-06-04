package fkevent

import (
	"time"

	"github.com/cloudwego/eino/schema"
)

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
	EventUsage              EventType = "usage"
	EventDispatchProgress   EventType = "dispatch_progress"
)

// EventPhase 描述事件在生命周期中的阶段。
type EventPhase string

const (
	EventPhaseStart    EventPhase = "start"
	EventPhaseDelta    EventPhase = "delta"
	EventPhaseComplete EventPhase = "complete"
	EventPhaseError    EventPhase = "error"
	EventPhaseInfo     EventPhase = "info"
)

// ContentKind 描述 content 字段的语义通道。
type ContentKind string

const (
	ContentKindOutput     ContentKind = "output"
	ContentKindReasoning  ContentKind = "reasoning"
	ContentKindToolArgs   ContentKind = "tool_args"
	ContentKindToolResult ContentKind = "tool_result"
	ContentKindError      ContentKind = "error"
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
	EventID          string            `json:"event_id,omitempty"`
	Sequence         int64             `json:"sequence,omitempty"`
	CreatedAt        time.Time         `json:"created_at,omitempty"`
	SpanID           string            `json:"span_id,omitempty"`
	ParentSpanID     string            `json:"parent_span_id,omitempty"`
	Type             EventType         `json:"type"`
	Phase            EventPhase        `json:"phase,omitempty"`
	IsPartial        bool              `json:"is_partial,omitempty"`
	IsFinal          bool              `json:"is_final,omitempty"`
	StreamID         string            `json:"stream_id,omitempty"`
	ChunkIndex       int64             `json:"chunk_index,omitempty"`
	ContentKind      ContentKind       `json:"content_kind,omitempty"`
	AgentName        string            `json:"agent_name,omitempty"`
	RunPath          string            `json:"run_path,omitempty"`
	Content          string            `json:"content,omitempty"`
	Detail           string            `json:"detail,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"` // 推理模型思考内容
	ToolCalls        []schema.ToolCall `json:"tool_calls,omitempty"`
	ToolCallSpanIDs  map[int]string    `json:"tool_call_span_ids,omitempty"` // 工具调用 index 到规范 span 的映射
	ToolCallRefs     map[int]string    `json:"tool_call_refs,omitempty"`     // 工具调用 index 到稳定引用的映射
	ToolCallRef      string            `json:"tool_call_ref,omitempty"`      // 当前工具生命周期的稳定引用
	ToolCallID       string            `json:"tool_call_id,omitempty"`       // 工具结果对应的调用 ID
	ExternalCallID   string            `json:"external_call_id,omitempty"`   // 模型/Provider 返回的原始调用 ID
	ToolName         string            `json:"tool_name,omitempty"`
	ToolCallIndex    *int              `json:"tool_call_index,omitempty"`
	IsMemberEvent    bool              `json:"is_member_event,omitempty"`
	MemberCallID     string            `json:"member_call_id,omitempty"`
	MemberToolName   string            `json:"member_tool_name,omitempty"`
	MemberName       string            `json:"member_name,omitempty"`
	MemberOrder      *int              `json:"member_order,omitempty"`
	ParentToolCallID string            `json:"parent_tool_call_id,omitempty"`
	ParentToolName   string            `json:"parent_tool_name,omitempty"`
	ActionType       ActionType        `json:"action_type,omitempty"`
	Error            string            `json:"error,omitempty"`
	PromptTokens     int               `json:"prompt_tokens,omitempty"`
	CompletionTokens int               `json:"completion_tokens,omitempty"`
	TotalTokens      int               `json:"total_tokens,omitempty"`
}

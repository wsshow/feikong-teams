package event

import (
	"fkteams/internal/domain/message"
	"time"
)

type Type string

const (
	TypeAgentStarted       Type = "agent_started"
	TypeAgentCompleted     Type = "agent_completed"
	TypeTurnStarted        Type = "turn_started"
	TypeTurnCompleted      Type = "turn_completed"
	TypeTurnFailed         Type = "turn_failed"
	TypeTurnCancelled      Type = "turn_cancelled"
	TypeUserMessage        Type = "user_message"
	TypeAssistantStarted   Type = "assistant_started"
	TypeAssistantReasoning Type = "assistant_reasoning_delta"
	TypeAssistantText      Type = "assistant_text_delta"
	TypeAssistantCompleted Type = "assistant_completed"
	TypeToolCallStarted    Type = "tool_call_started"
	TypeToolCallArguments  Type = "tool_call_arguments_delta"
	TypeToolCallResult     Type = "tool_call_result_delta"
	TypeToolCallCompleted  Type = "tool_call_completed"
	TypeToolCallFailed     Type = "tool_call_failed"
	TypeAskRequested       Type = "ask_requested"
	TypeAskAnswered        Type = "ask_answered"
	TypeApprovalRequested  Type = "approval_requested"
	TypeApprovalAnswered   Type = "approval_answered"
	TypeMemberStarted      Type = "member_started"
	TypeMemberCompleted    Type = "member_completed"
	TypeQueueUpdated       Type = "queue_updated"
	TypeSystemNotice       Type = "system_notice"
	TypeUsageReported      Type = "usage_reported"
	TypeError              Type = "error"
)

type DeltaKind string

const (
	DeltaOutput     DeltaKind = "output"
	DeltaReasoning  DeltaKind = "reasoning"
	DeltaToolArgs   DeltaKind = "tool_args"
	DeltaToolResult DeltaKind = "tool_result"
)

type NotifyType string

const (
	NotifyProcessingStart  NotifyType = "processing_start"
	NotifyProcessingEnd    NotifyType = "processing_end"
	NotifyUserMessage      NotifyType = "user_message"
	NotifyQueueUpdated     NotifyType = "queue_updated"
	NotifyCancelled        NotifyType = "cancelled"
	NotifyError            NotifyType = "error"
	NotifyApprovalRequired NotifyType = "approval_required"
	NotifyConnected        NotifyType = "connected"
	NotifyPong             NotifyType = "pong"
	NotifyInvalidAPIKey    NotifyType = "invalid_api_key"
)

type Event struct {
	EventID          string             `json:"event_id,omitempty"`
	Sequence         int64              `json:"sequence,omitempty"`
	CreatedAt        time.Time          `json:"created_at,omitempty"`
	Type             Type               `json:"type"`
	RunID            string             `json:"run_id,omitempty"`
	TurnID           string             `json:"turn_id,omitempty"`
	MessageID        string             `json:"message_id,omitempty"`
	BlockID          string             `json:"block_id,omitempty"`
	BlockType        string             `json:"block_type,omitempty"`
	ToolCallID       string             `json:"tool_call_id,omitempty"`
	ToolCallRef      string             `json:"tool_call_ref,omitempty"`
	ParentToolCallID string             `json:"parent_tool_call_id,omitempty"`
	ParentToolName   string             `json:"parent_tool_name,omitempty"`
	AgentName        string             `json:"agent_name,omitempty"`
	RunPath          string             `json:"run_path,omitempty"`
	Role             message.Role       `json:"role,omitempty"`
	DeltaKind        DeltaKind          `json:"delta_kind,omitempty"`
	Content          string             `json:"content,omitempty"`
	Detail           string             `json:"detail,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	Message          *message.Message   `json:"message,omitempty"`
	ToolCall         *message.ToolCall  `json:"tool_call,omitempty"`
	ToolCalls        []message.ToolCall `json:"tool_calls,omitempty"`
	ToolCallRefs     map[int]string     `json:"tool_call_refs,omitempty"`
	ToolName         string             `json:"tool_name,omitempty"`
	ToolArgs         string             `json:"tool_args,omitempty"`
	ToolResult       string             `json:"tool_result,omitempty"`
	ToolCallIndex    *int               `json:"tool_call_index,omitempty"`
	MemberCallID     string             `json:"member_call_id,omitempty"`
	MemberToolName   string             `json:"member_tool_name,omitempty"`
	MemberName       string             `json:"member_name,omitempty"`
	MemberOrder      *int               `json:"member_order,omitempty"`
	Error            string             `json:"error,omitempty"`
	PromptTokens     int                `json:"prompt_tokens,omitempty"`
	CompletionTokens int                `json:"completion_tokens,omitempty"`
	TotalTokens      int                `json:"total_tokens,omitempty"`
	Ask              *AskPayload        `json:"ask,omitempty"`
	Approval         *ApprovalPayload   `json:"approval,omitempty"`
	Usage            *UsagePayload      `json:"usage,omitempty"`
	Notice           *NoticePayload     `json:"notice,omitempty"`
}

type AskPayload struct {
	ID          string   `json:"id"`
	Question    string   `json:"question,omitempty"`
	Options     []string `json:"options,omitempty"`
	MultiSelect bool     `json:"multi_select,omitempty"`
	Selected    []string `json:"selected,omitempty"`
	FreeText    string   `json:"free_text,omitempty"`
}

type ApprovalPayload struct {
	ID       string `json:"id,omitempty"`
	Message  string `json:"message,omitempty"`
	Decision string `json:"decision,omitempty"`
}

type UsagePayload struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type NoticePayload struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

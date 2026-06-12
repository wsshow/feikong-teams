package agentcore

import (
	"context"
	"strings"
	"time"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type ContentPartType string

const (
	ContentPartText     ContentPartType = "text"
	ContentPartImageURL ContentPartType = "image_url"
	ContentPartAudioURL ContentPartType = "audio_url"
	ContentPartVideoURL ContentPartType = "video_url"
	ContentPartFileURL  ContentPartType = "file_url"
)

type ContentPart struct {
	Type       ContentPartType `json:"type"`
	Text       string          `json:"text,omitempty"`
	URL        string          `json:"url,omitempty"`
	Base64Data string          `json:"base64_data,omitempty"`
	MIMEType   string          `json:"mime_type,omitempty"`
	Detail     string          `json:"detail,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Index    *int         `json:"index,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

type Message struct {
	Role                  MessageRole   `json:"role"`
	Content               string        `json:"content,omitempty"`
	ReasoningContent      string        `json:"reasoning_content,omitempty"`
	ToolCalls             []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID            string        `json:"tool_call_id,omitempty"`
	ToolName              string        `json:"tool_name,omitempty"`
	UserInputMultiContent []ContentPart `json:"user_input_multi_content,omitempty"`
	MultiContent          []ContentPart `json:"multi_content,omitempty"`
	Name                  string        `json:"name,omitempty"`
}

type TurnInput struct {
	Context []Message
	Message Message
}

func (m Message) IsEmpty() bool {
	return m.Role == "" &&
		m.Content == "" &&
		m.ReasoningContent == "" &&
		len(m.ToolCalls) == 0 &&
		m.ToolCallID == "" &&
		m.ToolName == "" &&
		len(m.UserInputMultiContent) == 0 &&
		len(m.MultiContent) == 0 &&
		m.Name == ""
}

func (m Message) DisplayText() string {
	if strings.TrimSpace(m.Content) != "" {
		return m.Content
	}
	parts := m.UserInputMultiContent
	if len(parts) == 0 {
		parts = m.MultiContent
	}
	var texts []string
	for _, part := range parts {
		if part.Type == ContentPartText && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, " ")
}

func (input TurnInput) AllMessages() []Message {
	messages := make([]Message, 0, len(input.Context)+1)
	messages = append(messages, input.Context...)
	if !input.Message.IsEmpty() {
		messages = append(messages, input.Message)
	}
	return messages
}

type EventType string

const (
	EventAgentStart   EventType = "agent_start"
	EventAgentEnd     EventType = "agent_end"
	EventTurnStart    EventType = "turn_start"
	EventTurnEnd      EventType = "turn_end"
	EventMessageStart EventType = "message_start"
	EventMessageDelta EventType = "message_delta"
	EventMessageEnd   EventType = "message_end"
	EventToolStart    EventType = "tool_start"
	EventToolUpdate   EventType = "tool_update"
	EventToolEnd      EventType = "tool_end"
	EventAction       EventType = "action"
	EventUsage        EventType = "usage"
	EventError        EventType = "error"
	EventMemberUpdate EventType = "member_update"
)

type DeltaKind string

const (
	DeltaOutput     DeltaKind = "output"
	DeltaReasoning  DeltaKind = "reasoning"
	DeltaToolArgs   DeltaKind = "tool_args"
	DeltaToolResult DeltaKind = "tool_result"
)

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

type NotifyType string

const (
	NotifyProcessingStart  NotifyType = "processing_start"
	NotifyProcessingEnd    NotifyType = "processing_end"
	NotifyUserMessage      NotifyType = "user_message"
	NotifyQueueUpdated     NotifyType = "queue_updated"
	NotifyCancelled        NotifyType = "cancelled"
	NotifyError            NotifyType = "error"
	NotifyAskQuestions     NotifyType = "ask_questions"
	NotifyApprovalRequired NotifyType = "approval_required"
	NotifyConnected        NotifyType = "connected"
	NotifyPong             NotifyType = "pong"
	NotifyInvalidAPIKey    NotifyType = "invalid_api_key"
)

type Event struct {
	EventID          string         `json:"event_id,omitempty"`
	Sequence         int64          `json:"sequence,omitempty"`
	CreatedAt        time.Time      `json:"created_at,omitempty"`
	Type             EventType      `json:"type"`
	RunID            string         `json:"run_id,omitempty"`
	TurnID           string         `json:"turn_id,omitempty"`
	MessageID        string         `json:"message_id,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ToolCallRef      string         `json:"tool_call_ref,omitempty"`
	ParentToolCallID string         `json:"parent_tool_call_id,omitempty"`
	ParentToolName   string         `json:"parent_tool_name,omitempty"`
	AgentName        string         `json:"agent_name,omitempty"`
	RunPath          string         `json:"run_path,omitempty"`
	Role             MessageRole    `json:"role,omitempty"`
	DeltaKind        DeltaKind      `json:"delta_kind,omitempty"`
	Content          string         `json:"content,omitempty"`
	Detail           string         `json:"detail,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	Message          *Message       `json:"message,omitempty"`
	ToolCall         *ToolCall      `json:"tool_call,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallRefs     map[int]string `json:"tool_call_refs,omitempty"`
	ToolName         string         `json:"tool_name,omitempty"`
	ToolArgs         string         `json:"tool_args,omitempty"`
	ToolResult       string         `json:"tool_result,omitempty"`
	ToolCallIndex    *int           `json:"tool_call_index,omitempty"`
	MemberCallID     string         `json:"member_call_id,omitempty"`
	MemberToolName   string         `json:"member_tool_name,omitempty"`
	MemberName       string         `json:"member_name,omitempty"`
	MemberOrder      *int           `json:"member_order,omitempty"`
	ActionType       ActionType     `json:"action_type,omitempty"`
	Error            string         `json:"error,omitempty"`
	PromptTokens     int            `json:"prompt_tokens,omitempty"`
	CompletionTokens int            `json:"completion_tokens,omitempty"`
	TotalTokens      int            `json:"total_tokens,omitempty"`
}

type Interrupt struct {
	ID             string
	IsRootCause    bool
	Info           any
	MemberCallID   string
	MemberToolName string
	MemberName     string
	MemberOrder    *int
}

type InterruptHandler func(ctx context.Context, interrupts []Interrupt) (map[string]any, error)

type EventSink func(Event) error

type RunOptions struct {
	RunID            string
	CheckpointID     string
	Sink             EventSink
	InterruptHandler InterruptHandler
}

func (opts RunOptions) WithDefaults(defaultRunID string) RunOptions {
	if opts.RunID == "" {
		opts.RunID = opts.CheckpointID
	}
	if opts.RunID == "" {
		opts.RunID = defaultRunID
	}
	if opts.Sink == nil {
		opts.Sink = NoopEventSink
	}
	return opts
}

func NoopEventSink(Event) error {
	return nil
}

type RunResult struct {
	LastEvent Event
}

type Runner interface {
	Run(ctx context.Context, input TurnInput, opts RunOptions) (*RunResult, error)
}

type CheckPointStore interface {
	Set(ctx context.Context, key string, value []byte) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
}

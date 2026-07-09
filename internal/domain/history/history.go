package history

import (
	"fmt"
	"strings"
	"time"

	"fkteams/internal/domain/message"
)

type TranscriptEventType string

const (
	TranscriptUserMessage      TranscriptEventType = "user_message"
	TranscriptAgentStep        TranscriptEventType = "agent_step"
	TranscriptAssistantMessage TranscriptEventType = "assistant_message"
	TranscriptToolCallStart    TranscriptEventType = "tool_call_start"
	TranscriptToolCallEnd      TranscriptEventType = "tool_call_end"
	TranscriptAskRequested     TranscriptEventType = "ask_requested"
	TranscriptAskAnswered      TranscriptEventType = "ask_answered"
	TranscriptSystemNotice     TranscriptEventType = "system_notice"
	TranscriptError            TranscriptEventType = "error"
	TranscriptCancelled        TranscriptEventType = "cancelled"
)

type TranscriptEvent struct {
	ID            string                `json:"id"`
	At            time.Time             `json:"at"`
	Type          TranscriptEventType   `json:"type"`
	Agent         string                `json:"agent,omitempty"`
	CallID        string                `json:"call_id,omitempty"`
	Content       string                `json:"content,omitempty"`
	Detail        string                `json:"detail,omitempty"`
	Reasoning     string                `json:"reasoning,omitempty"`
	ContentParts  []message.ContentPart `json:"content_parts,omitempty"`
	Name          string                `json:"name,omitempty"`
	Args          string                `json:"args,omitempty"`
	Result        string                `json:"result,omitempty"`
	ResultRef     string                `json:"result_ref,omitempty"`
	Summary       string                `json:"summary,omitempty"`
	Truncated     bool                  `json:"truncated,omitempty"`
	OriginalChars int                   `json:"original_chars,omitempty"`
	Ask           *AskRecord            `json:"ask,omitempty"`
	Usage         *UsageRecord          `json:"usage,omitempty"`
	Error         *FriendlyError        `json:"error,omitempty"`
	Display       string                `json:"display,omitempty"`
	Kind          string                `json:"kind,omitempty"`
	Target        string                `json:"target,omitempty"`
}

type SubagentMetadata struct {
	AgentRunID   string `json:"agent_run_id,omitempty"`
	Agent        string `json:"agent"`
	ParentCallID string `json:"parent_call_id"`
	ToolName     string `json:"tool_name,omitempty"`
}

type ToolResultArtifact struct {
	ID            string    `json:"id"`
	ToolName      string    `json:"tool_name,omitempty"`
	Content       string    `json:"content"`
	Summary       string    `json:"summary,omitempty"`
	OriginalChars int       `json:"original_chars"`
	CreatedAt     time.Time `json:"created_at"`
}

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
	Status      string `json:"status,omitempty"`
}

type UsageRecord struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type AskRecord struct {
	ID          string   `json:"id,omitempty"`
	Question    string   `json:"question,omitempty"`
	Options     []string `json:"options,omitempty"`
	MultiSelect bool     `json:"multi_select,omitempty"`
	Selected    []string `json:"selected,omitempty"`
	FreeText    string   `json:"free_text,omitempty"`
	Answered    bool     `json:"answered,omitempty"`
}

type FriendlyError struct {
	Code            string   `json:"code,omitempty"`
	Title           string   `json:"title,omitempty"`
	Message         string   `json:"message,omitempty"`
	Suggestions     []string `json:"suggestions,omitempty"`
	TechnicalDetail string   `json:"technical_detail,omitempty"`
}

type MsgEventType string

const (
	MsgTypeText          MsgEventType = "text"
	MsgTypeReasoning     MsgEventType = "reasoning"
	MsgTypeToolCall      MsgEventType = "tool_call"
	MsgTypeAsk           MsgEventType = "ask"
	MsgTypeNotice        MsgEventType = "notice"
	MsgTypeUsageReported MsgEventType = "usage"
	MsgTypeError         MsgEventType = "error"
	MsgTypeCancelled     MsgEventType = "cancelled"
)

type MessageEvent struct {
	Type         MsgEventType          `json:"type"`
	EventID      string                `json:"event_id,omitempty"`
	Sequence     int64                 `json:"sequence,omitempty"`
	CreatedAt    time.Time             `json:"created_at,omitempty"`
	RunID        string                `json:"run_id,omitempty"`
	TurnID       string                `json:"turn_id,omitempty"`
	MessageID    string                `json:"message_id,omitempty"`
	Content      string                `json:"content,omitempty"`
	Detail       string                `json:"detail,omitempty"`
	ContentParts []message.ContentPart `json:"content_parts,omitempty"`
	Error        *FriendlyError        `json:"error,omitempty"`
	ToolCall     *ToolCallRecord       `json:"tool_call,omitempty"`
	Ask          *AskRecord            `json:"ask,omitempty"`
	Usage        *UsageRecord          `json:"usage,omitempty"`
}

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

func (m *AgentMessage) GetTextContent() string {
	var builder strings.Builder
	for _, event := range m.Events {
		if event.Type == MsgTypeText {
			builder.WriteString(event.Content)
		}
	}
	return builder.String()
}

func (m *AgentMessage) GetReasoningContent() string {
	var builder strings.Builder
	for _, event := range m.Events {
		if event.Type == MsgTypeReasoning {
			builder.WriteString(event.Content)
		}
	}
	return builder.String()
}

type AttachmentRef struct {
	ID           string              `json:"id"`
	MessageIndex int                 `json:"message_index"`
	EventIndex   int                 `json:"event_index"`
	PartIndex    int                 `json:"part_index"`
	AgentName    string              `json:"agent_name"`
	MessageText  string              `json:"message_text,omitempty"`
	StartTime    time.Time           `json:"start_time,omitempty"`
	Part         message.ContentPart `json:"part"`
}

func AttachmentID(messageIndex, eventIndex, partIndex int) string {
	return fmt.Sprintf("history:%06d:%02d:%02d", messageIndex, eventIndex, partIndex)
}

func ListAttachments(messages []AgentMessage) []AttachmentRef {
	var refs []AttachmentRef
	for msgIndex, msg := range messages {
		refs = append(refs, AttachmentsForMessage(msg, msgIndex)...)
	}
	return refs
}

func AttachmentsForMessage(msg AgentMessage, messageIndex int) []AttachmentRef {
	var refs []AttachmentRef
	messageText := strings.TrimSpace(msg.GetTextContent())
	for eventIndex, event := range msg.Events {
		for partIndex, part := range event.ContentParts {
			if !IsAttachmentPart(part) {
				continue
			}
			refs = append(refs, AttachmentRef{
				ID:           AttachmentID(messageIndex, eventIndex, partIndex),
				MessageIndex: messageIndex,
				EventIndex:   eventIndex,
				PartIndex:    partIndex,
				AgentName:    msg.AgentName,
				MessageText:  messageText,
				StartTime:    msg.StartTime,
				Part:         part,
			})
		}
	}
	return refs
}

func FindAttachment(messages []AgentMessage, id string) (AttachmentRef, bool) {
	for _, ref := range ListAttachments(messages) {
		if ref.ID == id {
			return ref, true
		}
	}
	return AttachmentRef{}, false
}

func IsAttachmentPart(part message.ContentPart) bool {
	return part.Type != "" && part.Type != message.ContentPartText
}

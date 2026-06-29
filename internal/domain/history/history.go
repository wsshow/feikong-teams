package history

import (
	"fmt"
	"strings"
	"time"

	"fkteams/internal/domain/message"
)

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

type Line struct {
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
	Sequence     int64                 `json:"sequence,omitempty"`
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

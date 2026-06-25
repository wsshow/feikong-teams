package message

import "strings"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
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
	Role             Role          `json:"role"`
	Content          string        `json:"content,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ToolName         string        `json:"tool_name,omitempty"`
	ContentParts     []ContentPart `json:"content_parts,omitempty"`
	Name             string        `json:"name,omitempty"`
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
		len(m.ContentParts) == 0 &&
		m.Name == ""
}

func (m Message) DisplayText() string {
	if strings.TrimSpace(m.Content) != "" {
		return m.Content
	}
	var texts []string
	for _, part := range m.ContentParts {
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

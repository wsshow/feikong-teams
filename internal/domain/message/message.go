package message

import (
	"reflect"
	"strings"
)

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
	Name       string          `json:"name,omitempty"`
	Text       string          `json:"text,omitempty"`
	URL        string          `json:"url,omitempty"`
	Base64Data string          `json:"base64_data,omitempty"`
	FileID     string          `json:"file_id,omitempty"`
	MIMEType   string          `json:"mime_type,omitempty"`
	Detail     string          `json:"detail,omitempty"`
	Extra      map[string]any  `json:"extra,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string         `json:"id"`
	Index    *int           `json:"index,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function FunctionCall   `json:"function"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// CloneMetadata 复制元数据中的嵌套映射和切片，隔离 SDK 对共享数据的修改。
func CloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	return cloneMetadataValue(reflect.ValueOf(metadata)).Interface().(map[string]any)
}

func cloneMetadataValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(cloneMetadataValue(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			result.SetMapIndex(iter.Key(), cloneMetadataValue(iter.Value()))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			result.Index(i).Set(cloneMetadataValue(value.Index(i)))
		}
		return result
	default:
		return value
	}
}

type Message struct {
	Role             Role           `json:"role"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ToolName         string         `json:"tool_name,omitempty"`
	ContentParts     []ContentPart  `json:"content_parts,omitempty"`
	Name             string         `json:"name,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

// Clone 返回独立消息副本，隔离工具、内容分段和供应商元数据的修改。
func (m Message) Clone() Message {
	m.Extra = CloneMetadata(m.Extra)
	if m.ToolCalls != nil {
		calls := append([]ToolCall{}, m.ToolCalls...)
		for i := range calls {
			calls[i].Extra = CloneMetadata(calls[i].Extra)
			if calls[i].Index != nil {
				index := *calls[i].Index
				calls[i].Index = &index
			}
		}
		m.ToolCalls = calls
	}
	if m.ContentParts != nil {
		parts := append([]ContentPart{}, m.ContentParts...)
		for i := range parts {
			parts[i].Extra = CloneMetadata(parts[i].Extra)
		}
		m.ContentParts = parts
	}
	return m
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
		len(m.Extra) == 0 &&
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

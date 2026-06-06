package eino

import (
	"fkteams/agentcore"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func adaptMessagesForRunner(messages []agentcore.Message) []adk.Message {
	result := make([]adk.Message, 0, len(messages))
	for _, msg := range messages {
		m := &schema.Message{
			Role:                  adaptRoleForRunner(msg.Role),
			Content:               msg.Content,
			ReasoningContent:      msg.ReasoningContent,
			ToolCallID:            msg.ToolCallID,
			ToolName:              msg.ToolName,
			Name:                  msg.Name,
			UserInputMultiContent: adaptPartsForRunner(msg.UserInputMultiContent),
		}
		if len(msg.MultiContent) > 0 {
			if msg.Role == agentcore.RoleAssistant {
				m.AssistantGenMultiContent = adaptOutputPartsForRunner(msg.MultiContent)
			} else {
				m.UserInputMultiContent = append(m.UserInputMultiContent, adaptPartsForRunner(msg.MultiContent)...)
			}
		}
		if len(msg.ToolCalls) > 0 {
			m.ToolCalls = adaptToolCallsForRunner(msg.ToolCalls)
		}
		result = append(result, m)
	}
	return result
}

func adaptMessageFromRunner(msg *schema.Message) agentcore.Message {
	if msg == nil {
		return agentcore.Message{}
	}
	return agentcore.Message{
		Role:                  adaptRoleFromRunner(msg.Role),
		Content:               msg.Content,
		ReasoningContent:      msg.ReasoningContent,
		ToolCalls:             adaptToolCallsFromRunner(msg.ToolCalls),
		ToolCallID:            msg.ToolCallID,
		ToolName:              msg.ToolName,
		UserInputMultiContent: adaptPartsFromRunner(msg.UserInputMultiContent),
		MultiContent:          adaptOutputPartsFromRunner(msg.AssistantGenMultiContent),
		Name:                  msg.Name,
	}
}

func adaptRoleForRunner(role agentcore.MessageRole) schema.RoleType {
	switch role {
	case agentcore.RoleSystem:
		return schema.System
	case agentcore.RoleUser:
		return schema.User
	case agentcore.RoleAssistant:
		return schema.Assistant
	case agentcore.RoleTool:
		return schema.Tool
	default:
		return schema.User
	}
}

func adaptRoleFromRunner(role schema.RoleType) agentcore.MessageRole {
	switch role {
	case schema.System:
		return agentcore.RoleSystem
	case schema.User:
		return agentcore.RoleUser
	case schema.Assistant:
		return agentcore.RoleAssistant
	case schema.Tool:
		return agentcore.RoleTool
	default:
		return agentcore.MessageRole(role)
	}
}

func adaptToolCallsForRunner(toolCalls []agentcore.ToolCall) []schema.ToolCall {
	result := make([]schema.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, schema.ToolCall{
			ID:    tc.ID,
			Index: tc.Index,
			Type:  tc.Type,
			Function: schema.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}

func adaptToolCallsFromRunner(toolCalls []schema.ToolCall) []agentcore.ToolCall {
	result := make([]agentcore.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, adaptToolCallFromRunner(tc))
	}
	return result
}

func adaptToolCallFromRunner(tc schema.ToolCall) agentcore.ToolCall {
	return agentcore.ToolCall{
		ID:    tc.ID,
		Index: tc.Index,
		Type:  tc.Type,
		Function: agentcore.FunctionCall{
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		},
	}
}

func adaptPartsForRunner(parts []agentcore.ContentPart) []schema.MessageInputPart {
	result := make([]schema.MessageInputPart, 0, len(parts))
	for _, part := range parts {
		p := schema.MessageInputPart{Text: part.Text}
		switch part.Type {
		case agentcore.ContentPartText:
			p.Type = schema.ChatMessagePartTypeText
		case agentcore.ContentPartImageURL:
			p.Type = schema.ChatMessagePartTypeImageURL
			p.Image = &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					URL:        stringPtr(part.URL),
					Base64Data: stringPtr(part.Base64Data),
					MIMEType:   part.MIMEType,
				},
				Detail: schema.ImageURLDetail(part.Detail),
			}
		case agentcore.ContentPartAudioURL:
			p.Type = schema.ChatMessagePartTypeAudioURL
			p.Audio = &schema.MessageInputAudio{MessagePartCommon: schema.MessagePartCommon{URL: stringPtr(part.URL)}}
		case agentcore.ContentPartVideoURL:
			p.Type = schema.ChatMessagePartTypeVideoURL
			p.Video = &schema.MessageInputVideo{MessagePartCommon: schema.MessagePartCommon{URL: stringPtr(part.URL)}}
		case agentcore.ContentPartFileURL:
			p.Type = schema.ChatMessagePartTypeFileURL
			p.File = &schema.MessageInputFile{MessagePartCommon: schema.MessagePartCommon{URL: stringPtr(part.URL)}}
		}
		result = append(result, p)
	}
	return result
}

func adaptPartsFromRunner(parts []schema.MessageInputPart) []agentcore.ContentPart {
	result := make([]agentcore.ContentPart, 0, len(parts))
	for _, part := range parts {
		p := agentcore.ContentPart{Text: part.Text}
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			p.Type = agentcore.ContentPartText
		case schema.ChatMessagePartTypeImageURL:
			p.Type = agentcore.ContentPartImageURL
			if part.Image != nil {
				if part.Image.URL != nil {
					p.URL = *part.Image.URL
				}
				if part.Image.Base64Data != nil {
					p.Base64Data = *part.Image.Base64Data
				}
				p.MIMEType = part.Image.MIMEType
				p.Detail = string(part.Image.Detail)
			}
		case schema.ChatMessagePartTypeAudioURL:
			p.Type = agentcore.ContentPartAudioURL
			if part.Audio != nil && part.Audio.URL != nil {
				p.URL = *part.Audio.URL
			}
		case schema.ChatMessagePartTypeVideoURL:
			p.Type = agentcore.ContentPartVideoURL
			if part.Video != nil && part.Video.URL != nil {
				p.URL = *part.Video.URL
			}
		case schema.ChatMessagePartTypeFileURL:
			p.Type = agentcore.ContentPartFileURL
			if part.File != nil && part.File.URL != nil {
				p.URL = *part.File.URL
			}
		}
		result = append(result, p)
	}
	return result
}

func adaptOutputPartsForRunner(parts []agentcore.ContentPart) []schema.MessageOutputPart {
	result := make([]schema.MessageOutputPart, 0, len(parts))
	for _, part := range parts {
		p := schema.MessageOutputPart{Type: schema.ChatMessagePartType(part.Type), Text: part.Text}
		switch part.Type {
		case agentcore.ContentPartImageURL:
			p.Image = &schema.MessageOutputImage{
				MessagePartCommon: schema.MessagePartCommon{
					URL:        stringPtr(part.URL),
					Base64Data: stringPtr(part.Base64Data),
					MIMEType:   part.MIMEType,
				},
			}
		case agentcore.ContentPartAudioURL:
			p.Audio = &schema.MessageOutputAudio{MessagePartCommon: schema.MessagePartCommon{URL: stringPtr(part.URL), Base64Data: stringPtr(part.Base64Data), MIMEType: part.MIMEType}}
		case agentcore.ContentPartVideoURL:
			p.Video = &schema.MessageOutputVideo{MessagePartCommon: schema.MessagePartCommon{URL: stringPtr(part.URL), Base64Data: stringPtr(part.Base64Data), MIMEType: part.MIMEType}}
		case agentcore.ContentPartFileURL:
			p.Extra = map[string]any{"url": part.URL, "mime_type": part.MIMEType}
		}
		result = append(result, p)
	}
	return result
}

func adaptOutputPartsFromRunner(parts []schema.MessageOutputPart) []agentcore.ContentPart {
	result := make([]agentcore.ContentPart, 0, len(parts))
	for _, part := range parts {
		p := agentcore.ContentPart{Type: agentcore.ContentPartType(part.Type), Text: part.Text}
		switch part.Type {
		case schema.ChatMessagePartTypeImageURL:
			p.Type = agentcore.ContentPartImageURL
			if part.Image != nil {
				if part.Image.URL != nil {
					p.URL = *part.Image.URL
				}
				if part.Image.Base64Data != nil {
					p.Base64Data = *part.Image.Base64Data
				}
				p.MIMEType = part.Image.MIMEType
			}
		case schema.ChatMessagePartTypeAudioURL:
			p.Type = agentcore.ContentPartAudioURL
			if part.Audio != nil {
				if part.Audio.URL != nil {
					p.URL = *part.Audio.URL
				}
				if part.Audio.Base64Data != nil {
					p.Base64Data = *part.Audio.Base64Data
				}
				p.MIMEType = part.Audio.MIMEType
			}
		case schema.ChatMessagePartTypeVideoURL:
			p.Type = agentcore.ContentPartVideoURL
			if part.Video != nil {
				if part.Video.URL != nil {
					p.URL = *part.Video.URL
				}
				if part.Video.Base64Data != nil {
					p.Base64Data = *part.Video.Base64Data
				}
				p.MIMEType = part.Video.MIMEType
			}
		case schema.ChatMessagePartTypeFileURL:
			p.Type = agentcore.ContentPartFileURL
			if part.Extra != nil {
				if url, ok := part.Extra["url"].(string); ok {
					p.URL = url
				}
				if mimeType, ok := part.Extra["mime_type"].(string); ok {
					p.MIMEType = mimeType
				}
			}
		}
		result = append(result, p)
	}
	return result
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

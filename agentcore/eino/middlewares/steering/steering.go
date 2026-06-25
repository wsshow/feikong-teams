package steering

import (
	"context"
	"fkteams/agentcore"
	einoruntime "fkteams/agentcore/eino"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func New() agentcore.AgentMiddleware {
	return einoruntime.WrapAgentMiddleware("steering", &handler{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
	})
}

type handler struct {
	*adk.BaseChatModelAgentMiddleware
}

func (h *handler) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	source, ok := agentcore.SteeringSourceFromContext(ctx)
	if !ok {
		return ctx, state, nil
	}
	messages, err := source(ctx)
	if err != nil {
		return ctx, nil, fmt.Errorf("consume steering: %w", err)
	}
	if len(messages) == 0 {
		return ctx, state, nil
	}

	next := *state
	next.Messages = append(append([]*schema.Message(nil), state.Messages...), adaptMessages(messages)...)
	return ctx, &next, nil
}

func adaptMessages(messages []agentcore.Message) []*schema.Message {
	result := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.IsEmpty() {
			continue
		}
		m := &schema.Message{
			Role:             adaptRole(msg.Role),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			ToolCallID:       msg.ToolCallID,
			ToolName:         msg.ToolName,
			Name:             msg.Name,
		}
		if len(msg.ContentParts) > 0 {
			m.UserInputMultiContent = adaptParts(msg.ContentParts)
		}
		result = append(result, m)
	}
	return result
}

func adaptRole(role agentcore.MessageRole) schema.RoleType {
	switch role {
	case agentcore.RoleSystem:
		return schema.System
	case agentcore.RoleAssistant:
		return schema.Assistant
	case agentcore.RoleTool:
		return schema.Tool
	default:
		return schema.User
	}
}

func adaptParts(parts []agentcore.ContentPart) []schema.MessageInputPart {
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
		default:
			p.Type = schema.ChatMessagePartTypeText
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

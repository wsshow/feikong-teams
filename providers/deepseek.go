package providers

import (
	"context"
	"encoding/json"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	aclOpenAI "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func newDeepSeekModel(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	m, err := openaiModel.NewChatModel(ctx, &openaiModel.ChatModelConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	})
	if err != nil {
		return nil, err
	}
	return &reasoningModel{inner: m}, nil
}

// reasoningModel 包装 ToolCallingChatModel，确保发送给 API 的 assistant 消息
// 包含 reasoning_content 字段（DeepSeek 思考模式要求）。
type reasoningModel struct {
	inner model.ToolCallingChatModel
}

func (m *reasoningModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	opts = append(opts, reasoningModifierOption(input))
	return m.inner.Generate(ctx, input, opts...)
}

func (m *reasoningModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
	*schema.StreamReader[*schema.Message], error) {
	opts = append(opts, reasoningModifierOption(input))
	return m.inner.Stream(ctx, input, opts...)
}

func (m *reasoningModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &reasoningModel{inner: inner}, nil
}

// reasoningModifierOption 创建 RequestPayloadModifier，为 JSON 请求体中的
// assistant 消息注入 reasoning_content 字段。
func reasoningModifierOption(msgs []*schema.Message) model.Option {
	return aclOpenAI.WithRequestPayloadModifier(
		func(_ context.Context, _ []*schema.Message, rawBody []byte) ([]byte, error) {
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rawBody, &body); err != nil {
				return rawBody, nil
			}

			rawMessages, ok := body["messages"]
			if !ok {
				return rawBody, nil
			}

			var jsonMsgs []map[string]json.RawMessage
			if err := json.Unmarshal(rawMessages, &jsonMsgs); err != nil {
				return rawBody, nil
			}

			reasoningMap := buildReasoningMap(msgs)

			modified := false
			assistantIdx := 0
			for i, m := range jsonMsgs {
				var role string
				if raw, ok := m["role"]; ok {
					json.Unmarshal(raw, &role)
				}
				if role != "assistant" {
					continue
				}
				if _, hasRC := m["reasoning_content"]; !hasRC {
					rc := ""
					if assistantIdx < len(reasoningMap) {
						rc = reasoningMap[assistantIdx]
					}
					encoded, _ := json.Marshal(rc)
					jsonMsgs[i]["reasoning_content"] = encoded
					modified = true
				}
				assistantIdx++
			}

			if !modified {
				return rawBody, nil
			}

			newMsgs, err := json.Marshal(jsonMsgs)
			if err != nil {
				return rawBody, nil
			}
			body["messages"] = newMsgs
			return json.Marshal(body)
		},
	)
}

// buildReasoningMap 按顺序提取所有 assistant 消息的 ReasoningContent。
func buildReasoningMap(msgs []*schema.Message) []string {
	var result []string
	for _, m := range msgs {
		if m.Role == schema.Assistant {
			result = append(result, m.ReasoningContent)
		}
	}
	return result
}

package model

import (
	"context"
	"fmt"

	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

// WithVisionSupport 按用户配置限制图片输入，未设置或支持时保留供应商行为。
func WithVisionSupport(inner runtimeport.ChatModel, supported *bool, modelName string) runtimeport.ChatModel {
	if supported == nil || *supported {
		return inner
	}
	return &textOnlyModel{inner: inner, modelName: modelName}
}

type textOnlyModel struct {
	inner     runtimeport.ChatModel
	modelName string
}

func (m *textOnlyModel) validate(input []message.Message) error {
	for _, msg := range input {
		for _, part := range msg.ContentParts {
			if part.Type == message.ContentPartImageURL {
				return fmt.Errorf("model %q does not support image_url input (supports_vision=false)", m.modelName)
			}
		}
	}
	return nil
}

func (m *textOnlyModel) Generate(ctx context.Context, input []message.Message) (message.Message, error) {
	if err := m.validate(input); err != nil {
		return message.Message{}, err
	}
	return m.inner.Generate(ctx, input)
}

func (m *textOnlyModel) Stream(ctx context.Context, input []message.Message) (runtimeport.MessageStream, error) {
	if err := m.validate(input); err != nil {
		return nil, err
	}
	return m.inner.Stream(ctx, input)
}

func (m *textOnlyModel) WithTools(tools []runtimeport.ToolInfo) (runtimeport.ChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &textOnlyModel{inner: inner, modelName: m.modelName}, nil
}

package providers

import (
	"context"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

func newOpenAIModel(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	return openaiModel.NewChatModel(ctx, &openaiModel.ChatModelConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	})
}

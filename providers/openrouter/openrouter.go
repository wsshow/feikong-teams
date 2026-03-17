package openrouter

import (
	"context"

	openrouterModel "github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino/components/model"
)

// New 创建 OpenRouter 的聊天模型
func New(ctx context.Context, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	cfg := &openrouterModel.Config{
		APIKey: apiKey,
		Model:  modelName,
	}
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return openrouterModel.NewChatModel(ctx, cfg)
}

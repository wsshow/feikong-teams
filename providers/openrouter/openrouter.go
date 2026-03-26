package openrouter

import (
	"context"

	openrouterModel "github.com/cloudwego/eino-ext/components/model/openrouter"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 OpenRouter 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	modelCfg := &openrouterModel.Config{
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	if cfg.BaseURL != "" {
		modelCfg.BaseURL = cfg.BaseURL
	}
	return openrouterModel.NewChatModel(ctx, modelCfg)
}

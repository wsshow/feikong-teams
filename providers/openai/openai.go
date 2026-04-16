package openai

import (
	"context"

	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 OpenAI 及兼容 API 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	modelCfg := &ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	return NewChatModel(ctx, modelCfg)
}

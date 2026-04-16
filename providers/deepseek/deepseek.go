package deepseek

import (
	"context"

	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 DeepSeek 原生 API 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	return NewChatModel(ctx, &ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	})
}

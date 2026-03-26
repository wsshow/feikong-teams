package deepseek

import (
	"context"

	deepseekModel "github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 DeepSeek 原生 API 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	return deepseekModel.NewChatModel(ctx, &deepseekModel.ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	})
}

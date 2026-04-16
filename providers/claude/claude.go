package claude

import (
	"context"

	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 Anthropic Claude 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	modelCfg := &Config{
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	if cfg.BaseURL != "" {
		modelCfg.BaseURL = &cfg.BaseURL
	}
	return NewChatModel(ctx, modelCfg)
}

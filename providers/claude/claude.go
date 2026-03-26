package claude

import (
	"context"

	claudeModel "github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 Anthropic Claude 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	modelCfg := &claudeModel.Config{
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	if cfg.BaseURL != "" {
		modelCfg.BaseURL = &cfg.BaseURL
	}
	return claudeModel.NewChatModel(ctx, modelCfg)
}

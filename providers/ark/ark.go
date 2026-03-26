package ark

import (
	"context"

	arkModel "github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建火山引擎方舟的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	return arkModel.NewChatModel(ctx, &arkModel.ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	})
}

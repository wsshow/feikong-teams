package ark

import (
	"context"

	arkModel "github.com/cloudwego/eino-ext/components/model/ark"

	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/providers/providerkit"
)

// New 创建火山引擎方舟的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (agentcore.ChatModel, error) {
	chatModel, err := arkModel.NewChatModel(ctx, &arkModel.ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: providerkit.HTTPClientWithHeaders(cfg.ExtraHeaders),
	})
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(chatModel), nil
}

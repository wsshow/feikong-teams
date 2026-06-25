package openrouter

import (
	"context"

	openrouterModel "github.com/cloudwego/eino-ext/components/model/openrouter"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/providers/providerkit"
)

// New 创建 OpenRouter 的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error) {
	modelCfg := &openrouterModel.Config{
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: providerkit.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	if cfg.BaseURL != "" {
		modelCfg.BaseURL = cfg.BaseURL
	}
	chatModel, err := openrouterModel.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(chatModel), nil
}

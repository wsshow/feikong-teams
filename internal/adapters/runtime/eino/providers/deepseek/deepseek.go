package deepseek

import (
	"context"

	deepseekModel "github.com/cloudwego/eino-ext/components/model/deepseek"

	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/providers/providerkit"
)

// New 创建 DeepSeek 原生 API 的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (agentcore.ChatModel, error) {
	chatModel, err := deepseekModel.NewChatModel(ctx, &deepseekModel.ChatModelConfig{
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

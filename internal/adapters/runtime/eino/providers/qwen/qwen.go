package qwen

import (
	"context"

	qwenModel "github.com/cloudwego/eino-ext/components/model/qwen"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/providers/providerkit"
)

// New 创建阿里通义千问的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error) {
	chatModel, err := qwenModel.NewChatModel(ctx, &qwenModel.ChatModelConfig{
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

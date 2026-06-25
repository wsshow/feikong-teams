package openai

import (
	"context"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/providers/providerkit"
)

// New 创建 OpenAI 及兼容 API 的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error) {
	modelCfg := &openaiModel.ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: providerkit.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	chatModel, err := openaiModel.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(chatModel), nil
}

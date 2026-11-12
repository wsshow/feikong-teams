package deepseek

import (
	"context"

	deepseekModel "github.com/cloudwego/eino-ext/components/model/deepseek"

	"fkteams/internal/adapters/model/providers/providerkit"
	kitruntime "fkteams/internal/adapters/runtime/agentkit"
	"fkteams/internal/adapters/runtime/agentkit/providers/openai"
	runtimeport "fkteams/internal/ports/runtime"
)

// New 创建 DeepSeek 原生 API 的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error) {
	// 原生 SDK 只接受文本；用户明确启用视觉时，使用可传递图片的兼容接口。
	if cfg.SupportsVision != nil && *cfg.SupportsVision {
		compatible := *cfg
		if compatible.BaseURL == "" {
			compatible.BaseURL = "https://api.deepseek.com/v1"
		}
		return openai.New(ctx, &compatible)
	}
	chatModel, err := deepseekModel.NewChatModel(ctx, &deepseekModel.ChatModelConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: providerkit.HTTPClientWithHeaders(cfg.ExtraHeaders),
	})
	if err != nil {
		return nil, err
	}
	return kitruntime.WrapChatModel(chatModel), nil
}

package copilot

import (
	"context"
	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	rootcopilot "fkteams/providers/copilot"
	"fkteams/providers/providerkit"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
)

// New 创建 Copilot 聊天模型（OpenAI 兼容）
func New(ctx context.Context, cfg *providerkit.Config) (agentcore.ChatModel, error) {
	tm := rootcopilot.GetTokenManager()

	// 确保有有效 token
	if _, err := tm.GetToken(ctx); err != nil {
		return nil, err
	}

	modelCfg := &openaiModel.ChatModelConfig{
		BaseURL:    rootcopilot.BaseURL(),
		Model:      cfg.Model,
		HTTPClient: rootcopilot.NewHTTPClient(),
	}
	chatModel, err := openaiModel.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(chatModel), nil
}

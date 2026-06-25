package claude

import (
	"context"

	claudeModel "github.com/cloudwego/eino-ext/components/model/claude"

	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/providers/providerkit"
)

// New 创建 Anthropic Claude 的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (agentcore.ChatModel, error) {
	modelCfg := &claudeModel.Config{
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: providerkit.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	if cfg.BaseURL != "" {
		modelCfg.BaseURL = &cfg.BaseURL
	}
	chatModel, err := claudeModel.NewChatModel(ctx, modelCfg)
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(chatModel), nil
}

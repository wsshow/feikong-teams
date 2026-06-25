package gemini

import (
	"context"

	geminiModel "github.com/cloudwego/eino-ext/components/model/gemini"
	"google.golang.org/genai"

	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/providers/providerkit"
)

// New 创建 Google Gemini 的聊天模型
func New(ctx context.Context, cfg *providerkit.Config) (agentcore.ChatModel, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: providerkit.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, err
	}
	chatModel, err := geminiModel.NewChatModel(ctx, &geminiModel.Config{
		Client: client,
		Model:  cfg.Model,
	})
	if err != nil {
		return nil, err
	}
	return einoruntime.WrapChatModel(chatModel), nil
}

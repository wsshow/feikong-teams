package gemini

import (
	"context"

	geminiModel "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"

	"fkteams/providers/internal"
)

// New 创建 Google Gemini 的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	clientCfg := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	}
	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, err
	}
	return geminiModel.NewChatModel(ctx, &geminiModel.Config{
		Client: client,
		Model:  cfg.Model,
	})
}

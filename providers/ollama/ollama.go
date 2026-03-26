package ollama

import (
	"context"

	ollamaModel "github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

// New 创建 Ollama 本地模型的聊天模型
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	return ollamaModel.NewChatModel(ctx, &ollamaModel.ChatModelConfig{
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		HTTPClient: internal.HTTPClientWithHeaders(cfg.ExtraHeaders),
	})
}

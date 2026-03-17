package ollama

import (
	"context"

	ollamaModel "github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
)

// New 创建 Ollama 本地模型的聊天模型
func New(ctx context.Context, _, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	return ollamaModel.NewChatModel(ctx, &ollamaModel.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
	})
}

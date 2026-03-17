package openai

import (
	"context"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// New 创建 OpenAI 及兼容 API 的聊天模型
func New(ctx context.Context, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	return openaiModel.NewChatModel(ctx, &openaiModel.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
	})
}

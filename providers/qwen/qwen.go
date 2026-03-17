package qwen

import (
	"context"

	qwenModel "github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
)

// New 创建阿里通义千问的聊天模型
func New(ctx context.Context, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	return qwenModel.NewChatModel(ctx, &qwenModel.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
	})
}

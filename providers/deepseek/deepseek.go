package deepseek

import (
	"context"

	deepseekModel "github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
)

// New 创建 DeepSeek 原生 API 的聊天模型
func New(ctx context.Context, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	return deepseekModel.NewChatModel(ctx, &deepseekModel.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
	})
}

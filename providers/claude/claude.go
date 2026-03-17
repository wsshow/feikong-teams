package claude

import (
	"context"

	claudeModel "github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
)

// New 创建 Anthropic Claude 的聊天模型
func New(ctx context.Context, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	cfg := &claudeModel.Config{
		APIKey: apiKey,
		Model:  modelName,
	}
	if baseURL != "" {
		cfg.BaseURL = &baseURL
	}
	return claudeModel.NewChatModel(ctx, cfg)
}

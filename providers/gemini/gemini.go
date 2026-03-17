package gemini

import (
	"context"

	geminiModel "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// New 创建 Google Gemini 的聊天模型
func New(ctx context.Context, apiKey, _, modelName string) (model.ToolCallingChatModel, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	return geminiModel.NewChatModel(ctx, &geminiModel.Config{
		Client: client,
		Model:  modelName,
	})
}

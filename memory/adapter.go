package memory

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// einoLLMAdapter 将 Eino ChatModel 适配为 LLMClient 接口
type einoLLMAdapter struct {
	model model.BaseChatModel
}

// NewLLMClient 基于 Eino BaseChatModel 创建 LLMClient
func NewLLMClient(m model.BaseChatModel) LLMClient {
	return &einoLLMAdapter{model: m}
}

func (a *einoLLMAdapter) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := a.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage("You are a memory extraction assistant. Respond only in the requested format."),
		schema.UserMessage(prompt),
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

package common

import (
	"context"
	"log"
	"os"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

func NewChatModel() model.ToolCallingChatModel {
	maxTokens := 8192
	cm, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:    os.Getenv("FEIKONG_OPENAI_API_KEY"),
		BaseURL:   os.Getenv("FEIKONG_OPENAI_BASE_URL"),
		Model:     os.Getenv("FEIKONG_OPENAI_MODEL"),
		MaxTokens: &maxTokens,
	})
	if err != nil {
		log.Fatal(err)
	}
	return cm
}

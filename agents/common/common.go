package common

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

const (
	MaxIterations = 60
	MaxRetries    = 3
)

func NewChatModel() model.ToolCallingChatModel {
	cm, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:  os.Getenv("FEIKONG_OPENAI_API_KEY"),
		BaseURL: os.Getenv("FEIKONG_OPENAI_BASE_URL"),
		Model:   os.Getenv("FEIKONG_OPENAI_MODEL"),
	})
	if err != nil {
		log.Fatal(err)
	}
	return cm
}

func NewChatModelWithConfig(modelName, baseURL, apiKey string) model.ToolCallingChatModel {
	cm, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		log.Fatal(err)
	}
	return cm
}

func IsRetryAble(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	// 网络错误（超时、连接中断等）
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := err.Error()
	return strings.Contains(msg, "status code: 429") ||
		strings.Contains(msg, "status code: 500") ||
		strings.Contains(msg, "status code: 502") ||
		strings.Contains(msg, "status code: 503") ||
		strings.Contains(msg, "status code: 504") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "EOF")
}

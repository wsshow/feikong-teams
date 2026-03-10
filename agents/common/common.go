// Package common 提供智能体共用的模型创建和重试判断等基础功能
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
	// MaxIterations 智能体最大迭代次数
	MaxIterations = 60
	// MaxRetries 最大重试次数
	MaxRetries = 3
)

// NewChatModel 使用环境变量配置创建聊天模型
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

// NewChatModelWithConfig 使用指定配置创建聊天模型
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

// IsRetryAble 判断错误是否可重试（网络错误、限流等）
func IsRetryAble(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	// context 已被取消或超时，不应重试
	if ctx.Err() != nil {
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

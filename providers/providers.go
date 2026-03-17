// Package providers 提供统一的模型提供者抽象层，基于 CloudWeGo Eino 框架。
// 通过工厂注册表模式，支持 OpenAI、DeepSeek 等多种模型提供者，并可自动检测类型。
package providers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

// Type 模型提供者类型
type Type string

const (
	OpenAI   Type = "openai"   // OpenAI 及 OpenAI 兼容 API
	DeepSeek Type = "deepseek" // DeepSeek 原生 API
)

// Config 统一模型配置
type Config struct {
	Provider Type   // 提供者类型，为空时自动检测
	APIKey   string // API 密钥
	BaseURL  string // API 地址
	Model    string // 模型名称
}

// Factory 模型创建函数类型
type Factory func(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error)

var factories = map[Type]Factory{}

func init() {
	Register(OpenAI, newOpenAIModel)
	Register(DeepSeek, newDeepSeekModel)
}

// Register 注册提供者工厂函数
func Register(t Type, f Factory) {
	factories[t] = f
}

// NewChatModel 根据配置创建聊天模型
func NewChatModel(ctx context.Context, cfg *Config) (model.ToolCallingChatModel, error) {
	t := cfg.Provider
	if t == "" {
		t = Detect(cfg.BaseURL, cfg.Model)
	}

	f, ok := factories[t]
	if !ok {
		return nil, fmt.Errorf("未知的模型提供者: %s", t)
	}

	return f(ctx, cfg)
}

// NewChatModelFromEnv 从环境变量创建聊天模型
func NewChatModelFromEnv(ctx context.Context) (model.ToolCallingChatModel, error) {
	return NewChatModel(ctx, &Config{
		Provider: Type(os.Getenv("FEIKONG_PROVIDER")),
		APIKey:   os.Getenv("FEIKONG_OPENAI_API_KEY"),
		BaseURL:  os.Getenv("FEIKONG_OPENAI_BASE_URL"),
		Model:    os.Getenv("FEIKONG_OPENAI_MODEL"),
	})
}

// Detect 从 BaseURL 或模型名称自动检测提供者类型
func Detect(baseURL, modelName string) Type {
	lower := strings.ToLower(baseURL + " " + modelName)
	if strings.Contains(lower, "deepseek") {
		return DeepSeek
	}
	return OpenAI
}

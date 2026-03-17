// Package providers 提供统一的模型提供者抽象层，基于 CloudWeGo Eino 框架。
// 通过工厂注册表模式，支持多种模型提供者，并可自动检测类型。
// 新增提供者只需在对应子包中实现 New 函数，并在此处注册即可。
package providers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/ark"
	"fkteams/providers/claude"
	"fkteams/providers/deepseek"
	"fkteams/providers/gemini"
	"fkteams/providers/ollama"
	"fkteams/providers/openai"
	"fkteams/providers/openrouter"
	"fkteams/providers/qwen"
)

// Type 模型提供者类型
type Type string

const (
	OpenAI     Type = "openai"     // OpenAI 及 OpenAI 兼容 API
	DeepSeek   Type = "deepseek"   // DeepSeek 原生 API
	Claude     Type = "claude"     // Anthropic Claude
	Ollama     Type = "ollama"     // Ollama 本地模型
	Ark        Type = "ark"        // 火山引擎方舟
	Gemini     Type = "gemini"     // Google Gemini
	Qwen       Type = "qwen"       // 阿里通义千问
	OpenRouter Type = "openrouter" // OpenRouter
)

// Config 统一模型配置
type Config struct {
	Provider Type   // 提供者类型，为空时自动检测
	APIKey   string // API 密钥
	BaseURL  string // API 地址
	Model    string // 模型名称
}

// Factory 模型创建函数类型
type Factory func(ctx context.Context, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error)

var factories = map[Type]Factory{}

func init() {
	Register(OpenAI, openai.New)
	Register(DeepSeek, deepseek.New)
	Register(Claude, claude.New)
	Register(Ollama, ollama.New)
	Register(Ark, ark.New)
	Register(Gemini, gemini.New)
	Register(Qwen, qwen.New)
	Register(OpenRouter, openrouter.New)
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

	return f(ctx, cfg.APIKey, cfg.BaseURL, cfg.Model)
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

	switch {
	case strings.Contains(lower, "deepseek"):
		return DeepSeek
	case strings.Contains(lower, "anthropic") || strings.Contains(lower, "claude"):
		return Claude
	case strings.Contains(lower, "ollama") || strings.Contains(lower, "11434"):
		return Ollama
	case strings.Contains(lower, "volces.com") || strings.Contains(lower, "volcengine"):
		return Ark
	case strings.Contains(lower, "generativelanguage.googleapis.com") || strings.Contains(lower, "gemini"):
		return Gemini
	case strings.Contains(lower, "dashscope") || strings.Contains(lower, "qwen"):
		return Qwen
	case strings.Contains(lower, "openrouter"):
		return OpenRouter
	default:
		return OpenAI
	}
}

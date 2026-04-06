// Package providers 提供统一的模型提供者抽象层，基于 CloudWeGo Eino 框架。
// 通过工厂注册表模式，支持多种模型提供者，并可自动检测类型。
// 新增提供者只需在对应子包中实现 New 函数，并在此处注册即可。
package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/ark"
	"fkteams/providers/claude"
	"fkteams/providers/copilot"
	"fkteams/providers/deepseek"
	"fkteams/providers/gemini"
	"fkteams/providers/internal"
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
	Copilot    Type = "copilot"    // GitHub Copilot
)

// Config 统一模型配置
type Config struct {
	Provider     Type              // 提供者类型，为空时自动检测
	APIKey       string            // API 密钥
	BaseURL      string            // API 地址
	Model        string            // 模型名称
	ExtraHeaders map[string]string // 额外 HTTP 请求头（如网关认证等）
}

// Factory 模型创建函数类型
type Factory func(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error)

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
	Register(Copilot, copilot.New)
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

	return f(ctx, &internal.Config{
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		Model:        cfg.Model,
		ExtraHeaders: cfg.ExtraHeaders,
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
	case strings.Contains(lower, "copilot") || strings.Contains(lower, "githubcopilot"):
		return Copilot
	default:
		return OpenAI
	}
}

// ModelInfo 模型信息
type ModelInfo = internal.ModelInfo

// ModelLister 模型列表获取接口
type ModelLister func(ctx context.Context, cfg *internal.Config) ([]ModelInfo, error)

var modelListers = map[Type]ModelLister{}

func init() {
	RegisterModelLister(OpenAI, internal.ListOpenAIModels)
	RegisterModelLister(DeepSeek, internal.ListOpenAIModels)
	RegisterModelLister(Qwen, internal.ListOpenAIModels)
	RegisterModelLister(OpenRouter, internal.ListOpenAIModels)
	RegisterModelLister(Ollama, internal.ListOpenAIModels)
	RegisterModelLister(Ark, internal.ListOpenAIModels)
	RegisterModelLister(Copilot, copilot.ListModels)
}

// RegisterModelLister 注册模型列表获取函数
func RegisterModelLister(t Type, l ModelLister) {
	modelListers[t] = l
}

// defaultBaseURLs 各提供者的默认 API 地址（用户未配置 base_url 时使用）
var defaultBaseURLs = map[Type]string{
	OpenAI:     "https://api.openai.com/v1",
	DeepSeek:   "https://api.deepseek.com",
	Qwen:       "https://dashscope.aliyuncs.com/compatible-mode/v1",
	OpenRouter: "https://openrouter.ai/api/v1",
	Ollama:     "http://localhost:11434/v1",
	Ark:        "https://ark.cn-beijing.volces.com/api/v3",
}

// DefaultBaseURL 返回指定提供者的默认 API 地址
func DefaultBaseURL(t Type) string {
	return defaultBaseURLs[t]
}

// ListModels 获取指定提供者的可用模型列表
func ListModels(ctx context.Context, cfg *Config) ([]ModelInfo, error) {
	t := cfg.Provider
	if t == "" {
		t = Detect(cfg.BaseURL, cfg.Model)
	}

	l, ok := modelListers[t]
	if !ok {
		return nil, fmt.Errorf("提供者 %s 不支持模型列表查询", t)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURLs[t]
	}

	return l(ctx, &internal.Config{
		APIKey:       cfg.APIKey,
		BaseURL:      baseURL,
		Model:        cfg.Model,
		ExtraHeaders: cfg.ExtraHeaders,
	})
}

// ProviderInfo 提供者信息
type ProviderInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListProviders 返回所有已注册的提供者信息
func ListProviders() []ProviderInfo {
	return []ProviderInfo{
		{ID: string(OpenAI), Name: "OpenAI"},
		{ID: string(DeepSeek), Name: "DeepSeek"},
		{ID: string(Claude), Name: "Claude"},
		{ID: string(Gemini), Name: "Gemini"},
		{ID: string(Qwen), Name: "通义千问"},
		{ID: string(Ollama), Name: "Ollama"},
		{ID: string(OpenRouter), Name: "OpenRouter"},
		{ID: string(Ark), Name: "火山方舟"},
		{ID: string(Copilot), Name: "GitHub Copilot"},
	}
}

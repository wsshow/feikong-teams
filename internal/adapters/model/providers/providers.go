// Package providers 提供统一的模型提供者抽象层。
// 通过工厂注册表模式，支持多种模型提供者，并可自动检测类型。
// 新增提供者只需在对应子包中实现 New 函数，并在此处注册即可。
package providers

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"strings"
	"sync"

	"fkteams/internal/adapters/model/providers/copilot"
	"fkteams/internal/adapters/model/providers/providerkit"
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
type Factory func(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error)

type registryContextKey struct{}

// Registry 持有模型 provider 工厂和模型列表查询器。
type Registry struct {
	mu           sync.RWMutex
	factories    map[Type]Factory
	modelListers map[Type]ModelLister
}

// NewRegistry 创建空模型 provider 注册表。
func NewRegistry() *Registry {
	return &Registry{
		factories:    make(map[Type]Factory),
		modelListers: make(map[Type]ModelLister),
	}
}

// WithRegistry 将模型 provider 注册表注入上下文。
func WithRegistry(ctx context.Context, registry *Registry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if registry == nil {
		return ctx
	}
	return context.WithValue(ctx, registryContextKey{}, registry)
}

// RegistryFromContext 从上下文读取模型 provider 注册表。
func RegistryFromContext(ctx context.Context) (*Registry, bool) {
	if ctx == nil {
		return nil, false
	}
	registry, ok := ctx.Value(registryContextKey{}).(*Registry)
	return registry, ok && registry != nil
}

// RequireRegistry 从上下文读取模型 provider 注册表，缺失时返回明确错误。
func RequireRegistry(ctx context.Context) (*Registry, error) {
	if registry, ok := RegistryFromContext(ctx); ok {
		return registry, nil
	}
	return nil, fmt.Errorf("model provider registry is not configured")
}

// Register 注册提供者工厂函数
func (r *Registry) Register(t Type, f Factory) {
	if r == nil || f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[t] = f
}

// NewChatModel 根据配置创建聊天模型
func (r *Registry) NewChatModel(ctx context.Context, cfg *Config) (runtimeport.ChatModel, error) {
	if r == nil {
		return nil, fmt.Errorf("model provider registry is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("model provider config is nil")
	}
	t := cfg.Provider
	if t == "" {
		t = Detect(cfg.BaseURL, cfg.Model)
	}

	r.mu.RLock()
	f := r.factories[t]
	r.mu.RUnlock()
	if f == nil {
		return nil, fmt.Errorf("未知的模型提供者: %s", t)
	}

	chatModel, err := f(ctx, &providerkit.Config{
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		Model:        cfg.Model,
		ExtraHeaders: cfg.ExtraHeaders,
	})
	if err != nil {
		return nil, err
	}
	return chatModel, nil
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
type ModelInfo = providerkit.ModelInfo

// ModelLister 模型列表获取接口
type ModelLister func(ctx context.Context, cfg *providerkit.Config) ([]ModelInfo, error)

// RegisterDefaultModelListers 注册内置模型列表查询实现。
func (r *Registry) RegisterDefaultModelListers() {
	r.RegisterModelLister(OpenAI, providerkit.ListOpenAIModels)
	r.RegisterModelLister(DeepSeek, providerkit.ListOpenAIModels)
	r.RegisterModelLister(Qwen, providerkit.ListOpenAIModels)
	r.RegisterModelLister(OpenRouter, providerkit.ListOpenAIModels)
	r.RegisterModelLister(Ollama, providerkit.ListOpenAIModels)
	r.RegisterModelLister(Ark, providerkit.ListOpenAIModels)
	r.RegisterModelLister(Copilot, copilot.ListModels)
}

// RegisterModelLister 注册模型列表获取函数
func (r *Registry) RegisterModelLister(t Type, l ModelLister) {
	if r == nil || l == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelListers[t] = l
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
func (r *Registry) ListModels(ctx context.Context, cfg *Config) ([]ModelInfo, error) {
	if r == nil {
		return nil, fmt.Errorf("model provider registry is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("model provider config is nil")
	}
	t := cfg.Provider
	if t == "" {
		t = Detect(cfg.BaseURL, cfg.Model)
	}

	r.mu.RLock()
	l := r.modelListers[t]
	r.mu.RUnlock()
	if l == nil {
		return nil, fmt.Errorf("提供者 %s 不支持模型列表查询", t)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURLs[t]
	}

	return l(ctx, &providerkit.Config{
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

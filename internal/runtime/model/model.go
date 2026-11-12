// Package model 提供运行时无关的模型创建注册表。
package model

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"strings"
	"sync"
)

// Type 模型提供者类型。
type Type string

const (
	OpenAI     Type = "openai"
	DeepSeek   Type = "deepseek"
	Claude     Type = "claude"
	Ollama     Type = "ollama"
	Ark        Type = "ark"
	Gemini     Type = "gemini"
	Qwen       Type = "qwen"
	OpenRouter Type = "openrouter"
	Copilot    Type = "copilot"
)

// Config 是创建聊天模型所需的最小配置。
type Config struct {
	Provider       Type
	APIKey         string
	BaseURL        string
	Model          string
	ExtraHeaders   map[string]string
	SupportsVision *bool
}

// Factory 创建运行时聊天模型。
type Factory func(ctx context.Context, cfg *Config) (runtimeport.ChatModel, error)

type registryContextKey struct{}

// Registry 保存一组运行时无关的模型工厂。
type Registry struct {
	mu        sync.RWMutex
	factories map[Type]Factory
}

// NewRegistry 创建空模型工厂注册表。
func NewRegistry() *Registry {
	return &Registry{factories: make(map[Type]Factory)}
}

// Register 注册模型提供者工厂。
func (r *Registry) Register(t Type, f Factory) {
	if r == nil || f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[t] = f
}

// NewChatModel 根据配置创建聊天模型。
func (r *Registry) NewChatModel(ctx context.Context, cfg *Config) (runtimeport.ChatModel, error) {
	if r == nil {
		return nil, fmt.Errorf("model registry is nil")
	}
	if cfg == nil {
		return nil, fmt.Errorf("model config is nil")
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
	chatModel, err := f(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return WithVisionSupport(chatModel, cfg.SupportsVision, cfg.Model), nil
}

// WithRegistry 将模型工厂注册表注入当前上下文。
func WithRegistry(ctx context.Context, registry *Registry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if registry == nil {
		return ctx
	}
	return context.WithValue(ctx, registryContextKey{}, registry)
}

// RegistryFromContext 从上下文读取模型工厂注册表。
func RegistryFromContext(ctx context.Context) (*Registry, bool) {
	if ctx == nil {
		return nil, false
	}
	registry, ok := ctx.Value(registryContextKey{}).(*Registry)
	return registry, ok && registry != nil
}

// RequireRegistry 从上下文读取模型工厂注册表，缺失时返回明确错误。
func RequireRegistry(ctx context.Context) (*Registry, error) {
	if registry, ok := RegistryFromContext(ctx); ok {
		return registry, nil
	}
	return nil, fmt.Errorf("model registry is not configured")
}

// Detect 从 BaseURL 或模型名称自动检测提供者类型。
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

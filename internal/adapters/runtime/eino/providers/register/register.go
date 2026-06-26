package register

import (
	"context"
	modelproviders "fkteams/internal/adapters/model/providers"
	"fkteams/internal/adapters/model/providers/providerkit"
	"fkteams/internal/adapters/runtime/eino/providers/ark"
	"fkteams/internal/adapters/runtime/eino/providers/claude"
	"fkteams/internal/adapters/runtime/eino/providers/copilot"
	"fkteams/internal/adapters/runtime/eino/providers/deepseek"
	"fkteams/internal/adapters/runtime/eino/providers/gemini"
	"fkteams/internal/adapters/runtime/eino/providers/ollama"
	"fkteams/internal/adapters/runtime/eino/providers/openai"
	"fkteams/internal/adapters/runtime/eino/providers/openrouter"
	"fkteams/internal/adapters/runtime/eino/providers/qwen"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
)

// RegisterDefaults 显式注册 Eino runtime 的内置模型提供者。
func RegisterDefaults(providerRegistry *modelproviders.Registry, runtimeRegistry *modelregistry.Registry) {
	if providerRegistry == nil {
		providerRegistry = modelproviders.NewRegistry()
	}
	providerRegistry.RegisterDefaultModelListers()

	providerRegistry.Register(modelproviders.OpenAI, openai.New)
	providerRegistry.Register(modelproviders.DeepSeek, deepseek.New)
	providerRegistry.Register(modelproviders.Claude, claude.New)
	providerRegistry.Register(modelproviders.Ollama, ollama.New)
	providerRegistry.Register(modelproviders.Ark, ark.New)
	providerRegistry.Register(modelproviders.Gemini, gemini.New)
	providerRegistry.Register(modelproviders.Qwen, qwen.New)
	providerRegistry.Register(modelproviders.OpenRouter, openrouter.New)
	providerRegistry.Register(modelproviders.Copilot, copilot.New)

	registerRuntimeModel(runtimeRegistry, modelregistry.OpenAI, openai.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.DeepSeek, deepseek.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.Claude, claude.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.Ollama, ollama.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.Ark, ark.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.Gemini, gemini.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.Qwen, qwen.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.OpenRouter, openrouter.New)
	registerRuntimeModel(runtimeRegistry, modelregistry.Copilot, copilot.New)
}

func registerRuntimeModel(registry *modelregistry.Registry, t modelregistry.Type, f func(context.Context, *providerkit.Config) (runtimeport.ChatModel, error)) {
	if registry == nil {
		return
	}
	registry.Register(t, func(ctx context.Context, cfg *modelregistry.Config) (runtimeport.ChatModel, error) {
		return f(ctx, &providerkit.Config{
			Provider:     string(cfg.Provider),
			APIKey:       cfg.APIKey,
			BaseURL:      cfg.BaseURL,
			Model:        cfg.Model,
			ExtraHeaders: cfg.ExtraHeaders,
		})
	})
}

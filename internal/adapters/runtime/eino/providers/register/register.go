package register

import (
	"fkteams/internal/adapters/runtime/eino/providers/ark"
	"fkteams/internal/adapters/runtime/eino/providers/claude"
	"fkteams/internal/adapters/runtime/eino/providers/copilot"
	"fkteams/internal/adapters/runtime/eino/providers/deepseek"
	"fkteams/internal/adapters/runtime/eino/providers/gemini"
	"fkteams/internal/adapters/runtime/eino/providers/ollama"
	"fkteams/internal/adapters/runtime/eino/providers/openai"
	"fkteams/internal/adapters/runtime/eino/providers/openrouter"
	"fkteams/internal/adapters/runtime/eino/providers/qwen"
	rootproviders "fkteams/providers"
)

func init() {
	rootproviders.Register(rootproviders.OpenAI, openai.New)
	rootproviders.Register(rootproviders.DeepSeek, deepseek.New)
	rootproviders.Register(rootproviders.Claude, claude.New)
	rootproviders.Register(rootproviders.Ollama, ollama.New)
	rootproviders.Register(rootproviders.Ark, ark.New)
	rootproviders.Register(rootproviders.Gemini, gemini.New)
	rootproviders.Register(rootproviders.Qwen, qwen.New)
	rootproviders.Register(rootproviders.OpenRouter, openrouter.New)
	rootproviders.Register(rootproviders.Copilot, copilot.New)
}

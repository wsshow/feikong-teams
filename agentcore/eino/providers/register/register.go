package register

import (
	"fkteams/agentcore/eino/providers/ark"
	"fkteams/agentcore/eino/providers/claude"
	"fkteams/agentcore/eino/providers/copilot"
	"fkteams/agentcore/eino/providers/deepseek"
	"fkteams/agentcore/eino/providers/gemini"
	"fkteams/agentcore/eino/providers/ollama"
	"fkteams/agentcore/eino/providers/openai"
	"fkteams/agentcore/eino/providers/openrouter"
	"fkteams/agentcore/eino/providers/qwen"
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

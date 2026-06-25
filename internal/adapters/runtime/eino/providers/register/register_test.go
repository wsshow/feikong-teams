package register

import (
	"context"
	"strings"
	"testing"

	rootproviders "fkteams/providers"
)

func TestInitRegistersAllProviderFactories(t *testing.T) {
	for _, provider := range []rootproviders.Type{
		rootproviders.OpenAI,
		rootproviders.DeepSeek,
		rootproviders.Claude,
		rootproviders.Ollama,
		rootproviders.Ark,
		rootproviders.Gemini,
		rootproviders.Qwen,
		rootproviders.OpenRouter,
		rootproviders.Copilot,
	} {
		t.Run(string(provider), func(t *testing.T) {
			_, err := rootproviders.NewChatModel(context.Background(), &rootproviders.Config{
				Provider: provider,
				BaseURL:  "http://127.0.0.1",
				Model:    "test-model",
			})
			if err != nil && strings.Contains(err.Error(), "未知的模型提供者") {
				t.Fatalf("%s factory was not registered: %v", provider, err)
			}
		})
	}
}

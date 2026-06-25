package providers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/testmodel"
	"fkteams/providers/providerkit"
)

func TestDetectProviderFromBaseURLOrModel(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		model    string
		expected Type
	}{
		{name: "deepseek", baseURL: "https://api.deepseek.com", expected: DeepSeek},
		{name: "claude", model: "claude-sonnet-4", expected: Claude},
		{name: "ollama", baseURL: "http://localhost:11434/v1", expected: Ollama},
		{name: "ark", baseURL: "https://ark.cn-beijing.volces.com/api/v3", expected: Ark},
		{name: "gemini", baseURL: "https://generativelanguage.googleapis.com/v1beta/openai", expected: Gemini},
		{name: "qwen", model: "qwen-plus", expected: Qwen},
		{name: "openrouter", baseURL: "https://openrouter.ai/api/v1", expected: OpenRouter},
		{name: "copilot", model: "githubcopilot/gpt-5", expected: Copilot},
		{name: "default", baseURL: "https://api.example.com/v1", model: "gpt-5", expected: OpenAI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect(tt.baseURL, tt.model); got != tt.expected {
				t.Fatalf("Detect(%q, %q) = %q, want %q", tt.baseURL, tt.model, got, tt.expected)
			}
		})
	}
}

func TestNewChatModelUsesFactoryAndAutoDetection(t *testing.T) {
	restoreProvidersGlobals(t)
	model := testmodel.New(testmodel.AssistantMessage("ok"))
	var captured *providerkit.Config
	Register(DeepSeek, func(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error) {
		captured = cfg
		return model, nil
	})

	got, err := NewChatModel(context.Background(), &Config{
		BaseURL:      "https://api.deepseek.com",
		APIKey:       "sk-test",
		Model:        "deepseek-chat",
		ExtraHeaders: map[string]string{"X-Test": "yes"},
	})
	if err != nil {
		t.Fatalf("NewChatModel() error = %v", err)
	}
	if got != model {
		t.Fatalf("NewChatModel() returned %#v, want registered model", got)
	}
	if captured == nil {
		t.Fatal("factory was not called")
	}
	if captured.APIKey != "sk-test" || captured.BaseURL != "https://api.deepseek.com" || captured.Model != "deepseek-chat" {
		t.Fatalf("captured config = %#v", captured)
	}
	if captured.ExtraHeaders["X-Test"] != "yes" {
		t.Fatalf("extra headers = %v, want X-Test", captured.ExtraHeaders)
	}
}

func TestNewChatModelReportsUnknownAndFactoryErrors(t *testing.T) {
	restoreProvidersGlobals(t)

	if _, err := NewChatModel(context.Background(), &Config{Provider: Type("missing")}); err == nil || !strings.Contains(err.Error(), "未知的模型提供者") {
		t.Fatalf("unknown provider error = %v", err)
	}

	Register(OpenAI, func(ctx context.Context, cfg *providerkit.Config) (runtimeport.ChatModel, error) {
		return nil, fmt.Errorf("factory failed")
	})
	if _, err := NewChatModel(context.Background(), &Config{Provider: OpenAI}); err == nil || !strings.Contains(err.Error(), "factory failed") {
		t.Fatalf("factory error = %v, want factory failed", err)
	}
}

func TestListModelsUsesListerAndDefaultBaseURL(t *testing.T) {
	restoreProvidersGlobals(t)
	var captured *providerkit.Config
	RegisterModelLister(OpenAI, func(ctx context.Context, cfg *providerkit.Config) ([]ModelInfo, error) {
		captured = cfg
		return []ModelInfo{{ID: "gpt-5"}}, nil
	})

	models, err := ListModels(context.Background(), &Config{
		Provider:     OpenAI,
		APIKey:       "sk-test",
		ExtraHeaders: map[string]string{"X-Gateway": "token"},
	})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if !reflect.DeepEqual(models, []ModelInfo{{ID: "gpt-5"}}) {
		t.Fatalf("models = %v, want gpt-5", models)
	}
	if captured == nil {
		t.Fatal("model lister was not called")
	}
	if captured.BaseURL != DefaultBaseURL(OpenAI) {
		t.Fatalf("baseURL = %q, want default %q", captured.BaseURL, DefaultBaseURL(OpenAI))
	}
	if captured.APIKey != "sk-test" || captured.ExtraHeaders["X-Gateway"] != "token" {
		t.Fatalf("captured config = %#v", captured)
	}
}

func TestListModelsReportsUnsupportedAndListerErrors(t *testing.T) {
	restoreProvidersGlobals(t)

	if _, err := ListModels(context.Background(), &Config{Provider: Type("missing")}); err == nil || !strings.Contains(err.Error(), "不支持模型列表查询") {
		t.Fatalf("unsupported provider error = %v", err)
	}

	RegisterModelLister(OpenAI, func(ctx context.Context, cfg *providerkit.Config) ([]ModelInfo, error) {
		return nil, fmt.Errorf("list failed")
	})
	if _, err := ListModels(context.Background(), &Config{Provider: OpenAI}); err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("lister error = %v, want list failed", err)
	}
}

func TestDefaultBaseURLAndListProviders(t *testing.T) {
	if DefaultBaseURL(OpenAI) != "https://api.openai.com/v1" {
		t.Fatalf("OpenAI default base URL = %q", DefaultBaseURL(OpenAI))
	}
	if DefaultBaseURL(Claude) != "" {
		t.Fatalf("Claude default base URL = %q, want empty", DefaultBaseURL(Claude))
	}

	providers := ListProviders()
	if len(providers) != 9 {
		t.Fatalf("provider count = %d, want 9", len(providers))
	}
	seen := map[string]string{}
	for _, provider := range providers {
		seen[provider.ID] = provider.Name
	}
	if seen[string(OpenAI)] != "OpenAI" || seen[string(Copilot)] != "GitHub Copilot" {
		t.Fatalf("providers = %v, missing expected names", providers)
	}
}

func restoreProvidersGlobals(t *testing.T) {
	t.Helper()

	originalFactories := factories
	originalModelListers := modelListers
	factories = map[Type]Factory{}
	modelListers = map[Type]ModelLister{}
	t.Cleanup(func() {
		factories = originalFactories
		modelListers = originalModelListers
	})
}

package handler

import (
	"context"
	"net/http"
	"testing"

	"fkteams/internal/adapters/model/providers"
	"fkteams/internal/adapters/model/providers/providerkit"
	"fkteams/internal/app/config"

	"github.com/gin-gonic/gin"
)

func TestGetProviderModelsRestoresSavedModelSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saveHandlerConfig(t, config.Config{
		Models: []config.ModelConfig{{
			ID:           "main",
			Name:         "主力模型",
			BaseURL:      "https://old.example/v1",
			APIKey:       "saved-key",
			ExtraHeaders: "X-Gateway: token",
		}},
	})

	registry := providers.NewRegistry()
	var captured *providerkit.Config
	registry.RegisterModelLister(providers.OpenAI, func(ctx context.Context, cfg *providerkit.Config) ([]providers.ModelInfo, error) {
		captured = cfg
		return []providers.ModelInfo{{ID: "gpt-5"}}, nil
	})

	router := gin.New()
	rt := NewRuntime(RuntimeOptions{Providers: registry})
	router.POST("/providers/models", rt.GetProviderModelsHandler())

	resp := performJSON(router, http.MethodPost, "/providers/models", `{
		"provider":"openai",
		"model_id":"renamed",
		"original_id":"main",
		"base_url":"https://new.example/v1",
		"extra_headers":"***"
	}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("provider models status = %d: %s", resp.Code, resp.Body.String())
	}
	if captured == nil {
		t.Fatal("model lister was not called")
	}
	if captured.APIKey != "saved-key" {
		t.Fatalf("api key = %q, want restored saved key", captured.APIKey)
	}
	if captured.BaseURL != "https://new.example/v1" {
		t.Fatalf("base url = %q", captured.BaseURL)
	}
	if captured.ExtraHeaders["X-Gateway"] != "token" {
		t.Fatalf("extra headers = %#v", captured.ExtraHeaders)
	}

	var models []providers.ModelInfo
	decodeRawData(t, resp, &models)
	if len(models) != 1 || models[0].ID != "gpt-5" {
		t.Fatalf("models = %#v", models)
	}
}

func TestRestoredModelAPIKeyRequiresStableModelIdentity(t *testing.T) {
	cfg := &config.Config{Models: []config.ModelConfig{{
		ID: "main", BaseURL: "https://shared.example/v1", APIKey: "saved-key",
	}}}
	if got := restoredModelAPIKey(cfg, "unknown", ""); got != "" {
		t.Fatalf("unknown model restored api key %q", got)
	}
	if got := restoredModelAPIKey(cfg, "main", ""); got != "saved-key" {
		t.Fatalf("known model api key = %q, want saved-key", got)
	}
}

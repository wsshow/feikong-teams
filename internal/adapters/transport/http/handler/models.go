package handler

import (
	"context"
	"net/http"
	"time"

	"fkteams/internal/adapters/model/providers"
	"fkteams/internal/app/config"

	"github.com/gin-gonic/gin"
)

// GetProvidersHandler 获取所有已注册的提供者信息

func (rt *Runtime) GetProvidersHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rt.Providers == nil {
			OK(c, []providers.ProviderInfo{})
			return
		}
		OK(c, rt.Providers.Providers())
	}
}

// GetProviderModelsHandler 获取指定提供者配置的可用模型列表

func (rt *Runtime) GetProviderModelsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Provider     string `json:"provider"`
			BaseURL      string `json:"base_url"`
			APIKey       string `json:"api_key"`
			ModelID      string `json:"model_id"`
			OriginalID   string `json:"original_id"`
			ExtraHeaders string `json:"extra_headers"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
			return
		}
		if req.Provider == "" {
			Fail(c, http.StatusBadRequest, "provider 不能为空")
			return
		}

		apiKey := req.APIKey
		if apiKey == "" {
			apiKey = restoredModelAPIKey(config.Get(), req.ModelID, req.OriginalID)
		}
		extraHeaders := req.ExtraHeaders
		if extraHeaders == sensitivePassword {
			extraHeaders = restoredModelExtraHeaders(config.Get(), req.ModelID, req.OriginalID)
		}
		modelCfg := config.ModelConfig{ExtraHeaders: extraHeaders}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		registry := rt.Providers
		if registry == nil {
			Fail(c, http.StatusServiceUnavailable, "model provider registry is not configured")
			return
		}

		models, err := registry.ListModels(ctx, &providers.Config{
			Provider:     providers.Type(req.Provider),
			BaseURL:      req.BaseURL,
			APIKey:       apiKey,
			ExtraHeaders: modelCfg.ParseExtraHeaders(),
		})
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		OK(c, models)
	}
}

func restoredModelAPIKey(cfg *config.Config, modelID, originalID string) string {
	if cfg == nil {
		return ""
	}
	if originalID != "" {
		if key := modelAPIKeyByID(cfg, originalID); key != "" {
			return key
		}
	}
	if modelID != "" {
		if key := modelAPIKeyByID(cfg, modelID); key != "" {
			return key
		}
	}
	return ""
}

func modelAPIKeyByID(cfg *config.Config, id string) string {
	for _, m := range cfg.Models {
		if m.ID == id && m.APIKey != "" {
			return m.APIKey
		}
	}
	return ""
}

func restoredModelExtraHeaders(cfg *config.Config, modelID, originalID string) string {
	if cfg == nil {
		return ""
	}
	if originalID != "" {
		if headers := modelExtraHeadersByID(cfg, originalID); headers != "" {
			return headers
		}
	}
	return modelExtraHeadersByID(cfg, modelID)
}

func modelExtraHeadersByID(cfg *config.Config, id string) string {
	for _, model := range cfg.Models {
		if model.ID == id {
			return model.ExtraHeaders
		}
	}
	return ""
}

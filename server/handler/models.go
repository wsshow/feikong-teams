package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"fkteams/config"
	"fkteams/providers"

	"github.com/gin-gonic/gin"
)

// GetProvidersHandler 获取所有已注册的提供者信息
func GetProvidersHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		OK(c, providers.ListProviders())
	}
}

// GetProviderModelsHandler 获取指定提供者配置的可用模型列表
func GetProviderModelsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Provider string `json:"provider"`
			BaseURL  string `json:"base_url"`
			APIKey   string `json:"api_key"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
			return
		}
		if req.Provider == "" {
			Fail(c, http.StatusBadRequest, "provider 不能为空")
			return
		}

		// 前端传入的 APIKey 是脱敏后的掩码值，需还原为真实密钥
		apiKey := req.APIKey
		if strings.Contains(apiKey, "****") {
			for _, m := range config.Get().Models {
				if m.Provider == req.Provider && m.APIKey != "" {
					apiKey = m.APIKey
					break
				}
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		models, err := providers.ListModels(ctx, &providers.Config{
			Provider: providers.Type(req.Provider),
			BaseURL:  req.BaseURL,
			APIKey:   apiKey,
		})
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		OK(c, models)
	}
}

package handler

import (
	"bytes"
	"encoding/json"
	"fkteams/config"
	"fkteams/providers"
	"fkteams/providers/copilot"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// OpenAIModelsHandler 返回所有已配置的模型列表（OpenAI 兼容格式）
func OpenAIModelsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		now := time.Now().Unix()

		type modelObject struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}

		models := make([]modelObject, 0, len(cfg.Models))
		for _, m := range cfg.Models {
			models = append(models, modelObject{
				ID:      m.Name,
				Object:  "model",
				Created: now,
				OwnedBy: "fkteams",
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   models,
		})
	}
}

// OpenAIChatCompletionsHandler 代理 chat/completions 请求到配置的模型后端
func OpenAIChatCompletionsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, openAIError("invalid_request_error", "failed to read request body"))
			return
		}

		// 解析 model 字段
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			c.JSON(http.StatusBadRequest, openAIError("invalid_request_error", "invalid JSON"))
			return
		}
		if req.Model == "" {
			c.JSON(http.StatusBadRequest, openAIError("invalid_request_error", "model is required"))
			return
		}

		// 查找模型配置
		cfg := config.Get()
		mc := cfg.ResolveModel(req.Model)
		if mc == nil {
			c.JSON(http.StatusNotFound, openAIError("model_not_found", fmt.Sprintf("model %q not found", req.Model)))
			return
		}

		// 确定后端 URL 和 HTTP 客户端
		pt := providers.Type(mc.Provider)
		if pt == "" {
			pt = providers.Detect(mc.BaseURL, mc.Model)
		}

		baseURL := mc.BaseURL
		if baseURL == "" {
			if pt == providers.Copilot {
				baseURL = copilot.BaseURL()
			} else {
				baseURL = providers.DefaultBaseURL(pt)
			}
		}
		if baseURL == "" {
			c.JSON(http.StatusInternalServerError, openAIError("server_error", "no base_url configured for model"))
			return
		}

		targetURL := strings.TrimRight(baseURL, "/") + "/chat/completions"

		// 替换请求中的 model 为实际模型名称
		var bodyMap map[string]any
		if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
			c.JSON(http.StatusBadRequest, openAIError("invalid_request_error", "invalid JSON"))
			return
		}
		bodyMap["model"] = mc.Model
		newBody, _ := json.Marshal(bodyMap)

		// 创建代理请求
		proxyReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", targetURL, bytes.NewReader(newBody))
		if err != nil {
			c.JSON(http.StatusInternalServerError, openAIError("server_error", "failed to create proxy request"))
			return
		}
		proxyReq.Header.Set("Content-Type", "application/json")

		// 选择 HTTP 客户端：Copilot 使用带 OAuth transport 的客户端，其余注入 API Key
		var client *http.Client
		if pt == providers.Copilot {
			client = copilot.NewHTTPClient()
		} else {
			if mc.APIKey != "" {
				proxyReq.Header.Set("Authorization", "Bearer "+mc.APIKey)
			}
			for k, v := range mc.ParseExtraHeaders() {
				proxyReq.Header.Set(k, v)
			}
			client = newProxyHTTPClient()
		}
		resp, err := client.Do(proxyReq)
		if err != nil {
			log.Printf("[openai-proxy] upstream request failed: model=%s, url=%s, err=%v", req.Model, targetURL, err)
			c.JSON(http.StatusBadGateway, openAIError("upstream_error", "upstream request failed"))
			return
		}
		defer resp.Body.Close()

		// 只透传安全的响应头，防止上游注入 Set-Cookie / Location 等
		safeHeaders := map[string]bool{
			"Content-Type":                   true,
			"X-Request-Id":                   true,
			"X-Ratelimit-Limit-Requests":     true,
			"X-Ratelimit-Limit-Tokens":       true,
			"X-Ratelimit-Remaining-Requests": true,
			"X-Ratelimit-Remaining-Tokens":   true,
			"X-Ratelimit-Reset-Requests":     true,
			"X-Ratelimit-Reset-Tokens":       true,
		}
		for k, vs := range resp.Header {
			if safeHeaders[k] {
				for _, v := range vs {
					c.Writer.Header().Add(k, v)
				}
			}
		}
		c.Writer.WriteHeader(resp.StatusCode)

		// 流式转发响应体
		if f, ok := c.Writer.(http.Flusher); ok {
			buf := make([]byte, 4096)
			for {
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					c.Writer.Write(buf[:n])
					f.Flush()
				}
				if readErr != nil {
					break
				}
			}
		} else {
			io.Copy(c.Writer, resp.Body)
		}
	}
}

func openAIError(errType, message string) gin.H {
	return gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	}
}

// newProxyHTTPClient 创建支持代理的 HTTP 客户端（长超时，适合 chat completions）
func newProxyHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := config.Get().ProxyURL(); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Timeout:   5 * time.Minute,
		Transport: transport,
	}
}

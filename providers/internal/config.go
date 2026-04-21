package internal

import (
	"context"
	"encoding/json"
	"fkteams/fkenv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config 统一模型配置（供各 provider 子包使用）
type Config struct {
	Provider     string            // 提供者类型
	APIKey       string            // API 密钥
	BaseURL      string            // API 地址
	Model        string            // 模型名称
	ExtraHeaders map[string]string // 额外 HTTP 请求头
}

// ModelInfo 模型信息（复用顶层结构）
type ModelInfo struct {
	ID string `json:"id"`
}

// ListOpenAIModels 通过 OpenAI 兼容的 /models 接口获取模型列表
func ListOpenAIModels(ctx context.Context, cfg *Config) ([]ModelInfo, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("未配置 base_url")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	modelsURL := baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		return nil, err
	}
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	for k, v := range cfg.ExtraHeaders {
		req.Header.Set(k, v)
	}

	client := NewHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求模型列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("该服务商不支持模型列表查询，请手动输入模型名称")
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("API Key 认证失败（%d），请检查密钥是否正确", resp.StatusCode)
		}
		return nil, fmt.Errorf("模型列表请求返回状态 %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{ID: m.ID})
	}
	return models, nil
}

// NewHTTPClient 创建支持代理的 HTTP 客户端
func NewHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := fkenv.Get(fkenv.ProxyURL); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
}

// HTTPClientWithHeaders 创建一个注入额外 HTTP Header 的客户端
// 如果 headers 为空，返回 nil（使用默认客户端）
func HTTPClientWithHeaders(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return nil
	}
	return &http.Client{
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			headers: headers,
		},
	}
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}

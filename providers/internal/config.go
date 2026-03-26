package internal

import "net/http"

// Config 统一模型配置（供各 provider 子包使用）
type Config struct {
	Provider     string            // 提供者类型
	APIKey       string            // API 密钥
	BaseURL      string            // API 地址
	Model        string            // 模型名称
	ExtraHeaders map[string]string // 额外 HTTP 请求头
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

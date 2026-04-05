package copilot

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sync"

	"github.com/google/uuid"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

var (
	// 全局 TokenManager 单例
	globalTM     *TokenManager
	globalTMOnce sync.Once
)

// GetTokenManager 返回全局 TokenManager 单例
func GetTokenManager() *TokenManager {
	globalTMOnce.Do(func() {
		globalTM = NewTokenManager()
	})
	return globalTM
}

// New 创建 Copilot 聊天模型（OpenAI 兼容）
func New(ctx context.Context, cfg *internal.Config) (model.ToolCallingChatModel, error) {
	tm := GetTokenManager()

	// 确保有有效 token
	if _, err := tm.GetToken(ctx); err != nil {
		return nil, err
	}

	modelCfg := &openaiModel.ChatModelConfig{
		BaseURL:    copilotBaseURL,
		Model:      cfg.Model,
		HTTPClient: newCopilotHTTPClient(tm),
	}
	return openaiModel.NewChatModel(ctx, modelCfg)
}

// newCopilotHTTPClient 创建带有 Copilot 认证和 X-Initiator 逻辑的 HTTP 客户端
// 支持 config.toml 中的 proxy.url 和 FEIKONG_PROXY_URL 环境变量代理
func newCopilotHTTPClient(tm *TokenManager) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := os.Getenv("FEIKONG_PROXY_URL"); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			base.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Transport: &copilotTransport{
			base: base,
			tm:   tm,
		},
	}
}

// copilotTransport 自定义 RoundTripper，负责：
// 1. 自动注入 Copilot 认证 headers
// 2. Token 过期自动刷新
// 3. 根据请求体内容设置 X-Initiator（控制计费）
type copilotTransport struct {
	base http.RoundTripper
	tm   *TokenManager
}

var assistantRolePattern = regexp.MustCompile(`"role"\s*:\s*"(?:assistant|tool)"`)

func (t *copilotTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 获取有效 token（自动刷新）
	token, err := t.tm.GetToken(req.Context())
	if err != nil {
		return nil, err
	}

	// 注入认证 header
	req.Header.Set("Authorization", "Bearer "+token)

	// 注入 Copilot 必须的 headers
	for k, v := range copilotHeaders() {
		req.Header.Set(k, v)
	}
	req.Header.Set("Openai-Intent", "conversation-panel")
	req.Header.Set("X-Request-Id", uuid.New().String())

	// 根据消息内容判断 X-Initiator
	initiator := "user"
	if req.Body != nil && req.Body != http.NoBody {
		bodyBytes, readErr := io.ReadAll(req.Body)
		req.Body.Close()
		if readErr == nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			if assistantRolePattern.Match(bodyBytes) {
				initiator = "agent"
			}
		}
	}
	req.Header.Set("X-Initiator", initiator)

	// 清除 SDK 可能自动设置的无效 API key header
	req.Header.Del("X-Api-Key")

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// 401 时尝试刷新 token 重试一次
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		t.tm.mu.Lock()
		if t.tm.token != nil {
			t.tm.token.ExpiresAt = 0 // 强制过期
		}
		t.tm.mu.Unlock()

		newToken, refreshErr := t.tm.GetToken(req.Context())
		if refreshErr != nil {
			return nil, refreshErr
		}
		req.Header.Set("Authorization", "Bearer "+newToken)

		// 重置 body
		if req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr == nil {
				req.Body = body
			}
		}
		return t.base.RoundTrip(req)
	}

	return resp, nil
}

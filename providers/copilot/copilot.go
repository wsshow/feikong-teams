package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"fkteams/providers/internal"
)

type contextKey int

const agentInitiatorKey contextKey = iota

func WithAgentInitiator(ctx context.Context) context.Context {
	return context.WithValue(ctx, agentInitiatorKey, true)
}

func isAgentInitiator(ctx context.Context) bool {
	v, ok := ctx.Value(agentInitiatorKey).(bool)
	return ok && v
}

// 全局会话 ID（进程级别，一次启动一个 session）
var sessionID = uuid.New().String()

// 全局设备 ID（持久化）
var deviceID string
var deviceIDOnce sync.Once

func getDeviceID() string {
	deviceIDOnce.Do(func() {
		deviceID = getOrCreateDeviceID()
	})
	return deviceID
}

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
func newCopilotHTTPClient(tm *TokenManager) *http.Client {
	return &http.Client{
		Transport: &copilotTransport{
			base: internal.NewHTTPClient().Transport,
			tm:   tm,
		},
	}
}

// NewHTTPClient 返回带有 Copilot 认证的 HTTP 客户端（供代理层复用）
func NewHTTPClient() *http.Client {
	return newCopilotHTTPClient(GetTokenManager())
}

// BaseURL 返回 Copilot API 基地址
func BaseURL() string {
	return copilotBaseURL
}

// copilotTransport 自定义 RoundTripper，负责：
// 1. 自动注入 Copilot 认证 headers
// 2. Token 过期自动刷新
// 3. 根据请求体内容设置 X-Initiator（控制计费）
type copilotTransport struct {
	base http.RoundTripper
	tm   *TokenManager
}

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

	// 会话和设备标识
	requestID := uuid.New().String()
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Agent-Task-Id", requestID)
	req.Header.Set("Vscode-Sessionid", sessionID)
	req.Header.Set("Editor-Device-Id", getDeviceID())

	// 读取请求体用于判断 X-Initiator 和 Vision
	var bodyBytes []byte
	if req.Body != nil && req.Body != http.NoBody {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// 根据上下文和消息内容判断计费分类
	if isAgentInitiator(req.Context()) {
		// agent 行为（记忆提取、摘要压缩、子任务等）：复用当前会话计费
		req.Header.Set("X-Initiator", "agent")
		req.Header.Set("Openai-Intent", "conversation-agent")
		req.Header.Set("X-Interaction-Type", "conversation-subagent")
	} else {
		// 用户发起或自动检测
		initiator := "user"
		if len(bodyBytes) > 0 {
			initiator = detectInitiator(bodyBytes)
		}
		req.Header.Set("X-Initiator", initiator)
		req.Header.Set("Openai-Intent", "conversation-agent")
		req.Header.Set("X-Interaction-Type", "conversation-agent")
	}

	// 交互分组 ID（用于将同一会话的请求归组）
	req.Header.Set("X-Interaction-Id", sessionID)

	// 检测 Vision 请求
	if len(bodyBytes) > 0 && detectVision(bodyBytes) {
		req.Header.Set("Copilot-Vision-Request", "true")
	}

	// 清除 SDK 可能自动设置的无效 header
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

// chatRequest 用于解析请求体中 messages 数组的最后一条消息 role
type chatRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
}

// detectInitiator 通过检查 messages 数组最后一条消息的 role 判断是否为 agent 发起
// 只有最后一条消息 role 为 "user" 时才标记为 "user"（新的用户意图），其余均为 "agent"
func detectInitiator(body []byte) string {
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil || len(req.Messages) == 0 {
		return "user"
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != "user" {
		return "agent"
	}
	return "user"
}

// detectVision 检查请求体中是否包含图片内容（image_url 类型的 content part）
func detectVision(body []byte) bool {
	return bytes.Contains(body, []byte(`"type":"image_url"`)) ||
		bytes.Contains(body, []byte(`"type": "image_url"`))
}

// ListModels 获取 Copilot 可用的模型列表
func ListModels(ctx context.Context, _ *internal.Config) ([]internal.ModelInfo, error) {
	tm := GetTokenManager()
	token, err := tm.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取 Copilot token 失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", copilotBaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range copilotHeaders() {
		req.Header.Set(k, v)
	}

	client := internal.NewHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Copilot 模型列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Copilot 模型列表返回状态 %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID                 string `json:"id"`
			ModelPickerEnabled bool   `json:"model_picker_enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []internal.ModelInfo
	for _, m := range result.Data {
		if m.ModelPickerEnabled {
			models = append(models, internal.ModelInfo{ID: m.ID})
		}
	}
	return models, nil
}

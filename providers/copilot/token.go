package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fkteams/common"
	"fkteams/providers/internal"
)

const (
	copilotTokenURL = "https://api.github.com/copilot_internal/v2/token"
	copilotBaseURL  = "https://api.githubcopilot.com"

	userAgent           = "GitHubCopilotChat/0.32.4"
	editorVersion       = "vscode/1.105.1"
	editorPluginVersion = "copilot-chat/0.32.4"
	integrationID       = "vscode-chat"
)

// Token 包含 GitHub OAuth token 和 Copilot API token
type Token struct {
	GitHubToken  string `json:"github_token"`  // 长期有效的 GitHub OAuth token
	CopilotToken string `json:"copilot_token"` // 短期有效的 Copilot API token
	ExpiresAt    int64  `json:"expires_at"`    // Copilot token 过期时间 (Unix timestamp)
}

// IsExpired 检查 Copilot token 是否已过期（提前 60 秒）
func (t *Token) IsExpired() bool {
	return time.Now().Unix() >= (t.ExpiresAt - 60)
}

// TokenManager 管理 Copilot token 的获取、缓存和刷新
type TokenManager struct {
	mu    sync.RWMutex
	token *Token
}

// NewTokenManager 创建 TokenManager 并从磁盘加载已保存的 token
func NewTokenManager() *TokenManager {
	tm := &TokenManager{}
	if t, err := loadTokenFromDisk(); err == nil {
		tm.token = t
	}
	return tm
}

// GetToken 获取有效的 Copilot token，过期时自动刷新
func (tm *TokenManager) GetToken(ctx context.Context) (string, error) {
	tm.mu.RLock()
	t := tm.token
	tm.mu.RUnlock()

	if t == nil {
		return "", fmt.Errorf("未登录 GitHub Copilot，请运行 'fkteams login copilot'")
	}

	if !t.IsExpired() {
		return t.CopilotToken, nil
	}

	// 需要刷新
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// double check
	if tm.token != nil && !tm.token.IsExpired() {
		return tm.token.CopilotToken, nil
	}

	newToken, err := exchangeCopilotToken(ctx, tm.token.GitHubToken)
	if err != nil {
		return "", fmt.Errorf("刷新 Copilot token 失败: %w", err)
	}

	tm.token = newToken
	_ = saveTokenToDisk(newToken)
	return newToken.CopilotToken, nil
}

// SetToken 设置新的 token 并持久化
func (tm *TokenManager) SetToken(t *Token) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.token = t
	return saveTokenToDisk(t)
}

// HasToken 检查是否已有 token
func (tm *TokenManager) HasToken() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.token != nil && tm.token.GitHubToken != ""
}

// exchangeCopilotToken 用 GitHub token 换取 Copilot API token
func exchangeCopilotToken(ctx context.Context, githubToken string) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", copilotTokenURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+githubToken)
	for k, v := range copilotHeaders() {
		req.Header.Set(k, v)
	}

	resp, err := internal.NewHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("此 GitHub 账号未订阅 Copilot")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("GitHub token 已失效，请重新登录")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取 Copilot token 失败，状态码: %d", resp.StatusCode)
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &Token{
		GitHubToken:  githubToken,
		CopilotToken: result.Token,
		ExpiresAt:    result.ExpiresAt,
	}, nil
}

// copilotHeaders 返回 Copilot API 必须的 HTTP 头
func copilotHeaders() map[string]string {
	return map[string]string{
		"User-Agent":             userAgent,
		"Editor-Version":         editorVersion,
		"Editor-Plugin-Version":  editorPluginVersion,
		"Copilot-Integration-Id": integrationID,
	}
}

// tokenFilePath 返回 token 持久化文件路径
func tokenFilePath() string {
	return filepath.Join(common.AppDir(), "copilot_token.json")
}

// saveTokenToDisk 将 token 保存到磁盘
func saveTokenToDisk(t *Token) error {
	dir := filepath.Dir(tokenFilePath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(tokenFilePath(), data, 0600)
}

// loadTokenFromDisk 从磁盘加载保存的 token
func loadTokenFromDisk() (*Token, error) {
	data, err := os.ReadFile(tokenFilePath())
	if err != nil {
		return nil, err
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if t.GitHubToken == "" {
		return nil, fmt.Errorf("token 文件内容无效")
	}
	return &t, nil
}

// ImportFromVSCode 尝试从 VS Code 已保存的 Copilot token 导入
func ImportFromVSCode() (string, bool) {
	data, err := os.ReadFile(vsCodeTokenPath())
	if err != nil {
		return "", false
	}

	var content map[string]struct {
		OAuthToken string `json:"oauth_token"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return "", false
	}

	key := "github.com:" + clientID
	if app, ok := content[key]; ok && app.OAuthToken != "" {
		return app.OAuthToken, true
	}
	return "", false
}

func vsCodeTokenPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "github-copilot", "apps.json")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "github-copilot", "apps.json")
	}
}

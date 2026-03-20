// Package common 提供各模块共用的工具函数和数据结构
package common

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/google/uuid"
)

const (
	// DefaultWorkspaceDir 默认工作目录
	DefaultWorkspaceDir = "./workspace"
	// MaxRetries 模型调用最大重试次数
	MaxRetries = 3
)

// GenerateSessionID 生成基于 UUID v4 的会话 ID
func GenerateSessionID() string {
	return uuid.New().String()
}

// WorkspaceDir 返回工作目录，优先使用 FEIKONG_WORKSPACE_DIR 环境变量
func WorkspaceDir() string {
	if d := os.Getenv("FEIKONG_WORKSPACE_DIR"); d != "" {
		return d
	}
	return DefaultWorkspaceDir
}

// GenerateExampleEnv 生成示例 .env 环境变量文件
func GenerateExampleEnv(filePath string) error {
	exampleContent := `# 这是一个示例的环境变量配置文件
# 请将此文件复制为 .env 并根据需要进行修改

# 模型配置配置
FEIKONG_OPENAI_BASE_URL = https://api.openai.com/v1
FEIKONG_OPENAI_API_KEY = xxxxx
FEIKONG_OPENAI_MODEL = GPT-5

# 模型提供者类型（可选，自动检测）: openai, deepseek, claude, ollama, ark, gemini, qwen, openrouter
# FEIKONG_PROVIDER = openai

# 配置代理：网络搜索工具、程序更新等
FEIKONG_PROXY_URL = http://127.0.0.1:7890

# 工作目录配置, 默认为: ./workspace
FEIKONG_WORKSPACE_DIR = ./workspace

# 代码助手
FEIKONG_CODER_ENABLED = true

# 本地命令行助手
FEIKONG_CMDER_ENABLED = true

# 个人全能助手（带审批以及子任务功能）
FEIKONG_ASSISTANT_ENABLED = true

# 数据分析师
FEIKONG_ANALYST_ENABLED = false

# 全局长期记忆
FEIKONG_MEMORY_ENABLED = false

# SSH 远程服务器配置
FEIKONG_SSH_VISITOR_ENABLED = false
FEIKONG_SSH_HOST =
FEIKONG_SSH_USERNAME = 
FEIKONG_SSH_PASSWORD =
`

	return os.WriteFile(filePath, []byte(exampleContent), 0644)
}

// IsRetryAble 判断错误是否可重试（网络错误、HTTP/2 stream 错误、限流等）
func IsRetryAble(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := err.Error()
	return strings.Contains(msg, "status code: 429") ||
		strings.Contains(msg, "status code: 500") ||
		strings.Contains(msg, "status code: 502") ||
		strings.Contains(msg, "status code: 503") ||
		strings.Contains(msg, "status code: 504") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "stream error") ||
		strings.Contains(msg, "INTERNAL_ERROR") ||
		strings.Contains(msg, "EOF")
}

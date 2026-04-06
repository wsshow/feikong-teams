// Package common 提供各模块共用的工具函数和数据结构
package common

import (
	"context"
	"errors"
	"fkteams/fkenv"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	// MaxRetries 模型调用最大重试次数
	MaxRetries = 3
	// defaultMaxIterations 默认最大迭代次数
	defaultMaxIterations = 60
)

// MaxIterations 返回智能体最大迭代次数，支持 FEIKONG_MAX_ITERATIONS 环境变量覆盖。
// 设为 0 或负数表示不限制
func MaxIterations() int {
	if v := fkenv.Get(fkenv.MaxIterations); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n <= 0 {
				return 1<<31 - 1
			}
			return n
		}
	}
	return defaultMaxIterations
}

// AppDir 返回应用数据目录 ~/.fkteams，支持 FEIKONG_APP_DIR 环境变量覆盖
func AppDir() string {
	if d := fkenv.Get(fkenv.AppDir); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".fkteams"
	}
	return filepath.Join(home, ".fkteams")
}

// SessionsDir 返回会话历史存储目录（CLI 与通道共用）
func SessionsDir() string {
	return filepath.Join(AppDir(), "sessions")
}

// GenerateSessionID 生成基于 UUID v4 的会话 ID
func GenerateSessionID() string {
	return uuid.New().String()
}

// WorkspaceDir 返回工作目录（固定为 ~/.fkteams/workspace）
func WorkspaceDir() string {
	return filepath.Join(AppDir(), "workspace")
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

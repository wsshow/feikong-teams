// Package common 提供智能体共用的模型创建和重试判断等基础功能
package common

import (
	"context"
	"errors"
	rootcommon "fkteams/common"
	"fkteams/providers"
	"net"
	"strings"

	"github.com/cloudwego/eino/components/model"
)

const (
	// MaxIterations 智能体最大迭代次数
	MaxIterations = 60
	// MaxRetries 最大重试次数
	MaxRetries = 3
)

// WorkspaceDir 返回工作目录，优先使用 FEIKONG_WORKSPACE_DIR 环境变量
func WorkspaceDir() string {
	return rootcommon.WorkspaceDir()
}

// NewChatModel 使用环境变量配置创建聊天模型
func NewChatModel() (model.ToolCallingChatModel, error) {
	return providers.NewChatModelFromEnv(context.Background())
}

// NewChatModelWithConfig 使用指定配置创建聊天模型
func NewChatModelWithConfig(cfg *providers.Config) (model.ToolCallingChatModel, error) {
	return providers.NewChatModel(context.Background(), cfg)
}

// IsRetryAble 判断错误是否可重试（网络错误、限流等）
func IsRetryAble(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	// context 已被取消或超时，不应重试
	if ctx.Err() != nil {
		return false
	}

	// 网络错误（超时、连接中断等）
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
		strings.Contains(msg, "EOF")
}

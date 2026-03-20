// Package common 提供智能体共用的模型创建和重试判断等基础功能
package common

import (
	"context"
	rootcommon "fkteams/common"
	"fkteams/providers"

	"github.com/cloudwego/eino/components/model"
)

const (
	// MaxIterations 智能体最大迭代次数
	MaxIterations = 60
	// MaxRetries 最大重试次数
	MaxRetries = rootcommon.MaxRetries
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

// IsRetryAble 判断错误是否可重试（转发到 common 包）
func IsRetryAble(ctx context.Context, err error) bool {
	return rootcommon.IsRetryAble(ctx, err)
}

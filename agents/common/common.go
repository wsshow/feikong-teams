// Package common 提供智能体共用的模型创建和重试判断等基础功能
package common

import (
	"context"
	rootcommon "fkteams/common"
	"fkteams/config"
	"fkteams/providers"
	"fmt"

	"github.com/cloudwego/eino/components/model"
)

// MaxIterations 返回智能体最大迭代次数
func MaxIterations() int {
	return rootcommon.MaxIterations()
}

const (
	// MaxRetries 最大重试次数
	MaxRetries = rootcommon.MaxRetries
)

// WorkspaceDir 返回工作目录
func WorkspaceDir() string {
	return config.Get().WorkspaceDir()
}

// NewChatModel 使用配置文件的 default 模型创建聊天模型
func NewChatModel() (model.ToolCallingChatModel, error) {
	cfg := config.Get()
	modelCfg := cfg.ResolveModel("default")
	if modelCfg != nil && (modelCfg.APIKey != "" || modelCfg.Provider != "") {
		return NewChatModelWithModelConfig(modelCfg)
	}
	return nil, fmt.Errorf("未配置默认模型，请运行 fkteams generate config 生成配置文件")
}

// NewChatModelWithModelConfig 使用 ModelConfig 创建聊天模型
func NewChatModelWithModelConfig(mc *config.ModelConfig) (model.ToolCallingChatModel, error) {
	return providers.NewChatModel(context.Background(), &providers.Config{
		Provider:     providers.Type(mc.Provider),
		APIKey:       mc.APIKey,
		BaseURL:      mc.BaseURL,
		Model:        mc.Model,
		ExtraHeaders: mc.ParseExtraHeaders(),
	})
}

// NewChatModelWithConfig 使用指定配置创建聊天模型
func NewChatModelWithConfig(cfg *providers.Config) (model.ToolCallingChatModel, error) {
	return providers.NewChatModel(context.Background(), cfg)
}

// IsRetryAble 判断错误是否可重试（转发到 common 包）
func IsRetryAble(ctx context.Context, err error) bool {
	return rootcommon.IsRetryAble(ctx, err)
}

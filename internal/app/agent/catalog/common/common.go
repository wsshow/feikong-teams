// Package common 提供智能体共用的模型创建和重试判断等基础功能
package common

import (
	"context"
	"fkteams/internal/app/config"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
	retry "fkteams/internal/runtime/retry"
	"fmt"
)

// MaxIterations 返回智能体最大迭代次数
func MaxIterations() int {
	return retry.MaxIterations()
}

// WorkspaceDir 返回工作目录
func WorkspaceDir() string {
	return config.Get().WorkspaceDir()
}

// NewChatModel 使用配置文件的默认对话模型创建聊天模型
func NewChatModel(ctx context.Context) (runtimeport.ChatModel, error) {
	cfg := config.Get()
	modelCfg := cfg.ResolveDefaultModel(config.ModelUseChat)
	if modelCfg != nil && (modelCfg.APIKey != "" || modelCfg.Provider != "") {
		return NewChatModelWithModelConfig(ctx, modelCfg)
	}
	return nil, fmt.Errorf("未配置默认模型，请运行 fkteams generate config 生成配置文件")
}

// NewChatModelWithModelConfig 使用 ModelConfig 创建聊天模型
func NewChatModelWithModelConfig(ctx context.Context, mc *config.ModelConfig) (runtimeport.ChatModel, error) {
	return NewChatModelWithConfig(ctx, &modelregistry.Config{
		Provider:       modelregistry.Type(mc.Provider),
		APIKey:         mc.APIKey,
		BaseURL:        mc.BaseURL,
		Model:          mc.Model,
		ExtraHeaders:   mc.ParseExtraHeaders(),
		SupportsVision: mc.SupportsVision,
	})
}

// NewChatModelWithConfig 使用指定配置创建聊天模型
func NewChatModelWithConfig(ctx context.Context, cfg *modelregistry.Config) (runtimeport.ChatModel, error) {
	registry, err := modelregistry.RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.NewChatModel(ctx, cfg)
}

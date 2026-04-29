package lifecycle

import (
	"fkteams/common"
	"fkteams/config"
	"os"
	"path/filepath"
	"syscall"
)

// AppConfig 应用配置
type AppConfig struct {
	WorkspaceDir       string // 工作目录
	MemoryEnabled      bool   // 是否启用长期记忆
	SchedulerEnabled   bool   // 是否启用定时任务
	SchedulerDir       string // 定时任务调度器数据目录
	InputHistoryPath   string // 输入历史文件路径
	ChatHistoryDir     string // 聊天历史目录

	// ExitSignals 触发退出的系统信号（CLI 模式应排除 SIGINT）
	ExitSignals []os.Signal
}

// DefaultConfig 返回基于配置文件的默认配置
func DefaultConfig() *AppConfig {
	appDir := common.AppDir()
	cfg := config.Get()
	return &AppConfig{
		WorkspaceDir:       cfg.WorkspaceDir(),
		MemoryEnabled:      cfg.Memory.Enabled,
		SchedulerEnabled:   true,
		SchedulerDir:       common.SchedulerDir(),
		InputHistoryPath:   filepath.Join(appDir, "history", "input_history", "fkteams_input_history"),
		ChatHistoryDir:     filepath.Join(appDir, "history", "chat_history"),
		ExitSignals:        []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP},
	}
}

// Option 自定义配置的选项函数
type Option func(*AppConfig)

// WithWorkspaceDir 设置工作目录
func WithWorkspaceDir(dir string) Option {
	return func(c *AppConfig) {
		c.WorkspaceDir = dir
	}
}

// WithMemoryEnabled 设置是否启用长期记忆
func WithMemoryEnabled(enabled bool) Option {
	return func(c *AppConfig) {
		c.MemoryEnabled = enabled
	}
}

// WithSchedulerEnabled 设置是否启用定时任务
func WithSchedulerEnabled(enabled bool) Option {
	return func(c *AppConfig) {
		c.SchedulerEnabled = enabled
	}
}

// WithSchedulerDir 设置定时任务调度器数据目录
func WithSchedulerDir(dir string) Option {
	return func(c *AppConfig) {
		c.SchedulerDir = dir
	}
}

// WithInputHistoryPath 设置输入历史文件路径
func WithInputHistoryPath(path string) Option {
	return func(c *AppConfig) {
		c.InputHistoryPath = path
	}
}

// WithChatHistoryDir 设置聊天历史目录
func WithChatHistoryDir(dir string) Option {
	return func(c *AppConfig) {
		c.ChatHistoryDir = dir
	}
}

// WithExitSignals 设置触发退出的信号列表
func WithExitSignals(signals ...os.Signal) Option {
	return func(c *AppConfig) {
		c.ExitSignals = signals
	}
}

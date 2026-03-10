package lifecycle

import (
	"os"
	"syscall"
)

// AppConfig 应用配置
type AppConfig struct {
	WorkspaceDir       string
	MemoryEnabled      bool
	SchedulerEnabled   bool
	SchedulerOutputDir string
	InputHistoryPath   string
	ChatHistoryDir     string

	// ExitSignals 触发退出的系统信号（CLI 模式应排除 SIGINT）
	ExitSignals []os.Signal
}

// DefaultConfig 返回基于环境变量的默认配置
func DefaultConfig() *AppConfig {
	workspaceDir := "./workspace"
	if d := os.Getenv("FEIKONG_WORKSPACE_DIR"); d != "" {
		workspaceDir = d
	}

	return &AppConfig{
		WorkspaceDir:       workspaceDir,
		MemoryEnabled:      os.Getenv("FEIKONG_MEMORY_ENABLED") == "true",
		SchedulerEnabled:   true,
		SchedulerOutputDir: "./result/scheduled_tasks/",
		InputHistoryPath:   "./history/input_history/fkteams_input_history",
		ChatHistoryDir:     "./history/chat_history/",
		ExitSignals:        []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP},
	}
}

// Option 用于自定义 AppConfig 的选项函数
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

// WithSchedulerOutputDir 设置定时任务输出目录
func WithSchedulerOutputDir(dir string) Option {
	return func(c *AppConfig) {
		c.SchedulerOutputDir = dir
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

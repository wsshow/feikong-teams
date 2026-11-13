// Package process 声明本地进程和任务持久化的运行时无关契约。
package process

import (
	"context"
	domain "fkteams/internal/domain/process"
)

// Spec 是已经过权限校验的进程启动参数。
type Spec struct{ ID, Command, Directory string }

// Output 使用绝对字节游标读取合并日志；From 大于请求游标表示旧日志已被轮转。
type Output struct {
	Text      string `json:"output,omitempty"`
	From      int64  `json:"from_cursor"`
	Next      int64  `json:"next_cursor"`
	Total     int64  `json:"total_bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Result 是进程退出及输出排空后的结果。
type Result struct {
	ExitCode       int
	Error          string
	OutputBytes    int64
	Stdout, Stderr string
}

// Process 的 Wait 只调用一次，取消由启动上下文传递。
type Process interface {
	PID() int
	Wait() Result
	Read(cursor int64, limit int) (Output, error)
}

// Runner 创建进程并管理有界日志；恢复读取不依赖原来的进程句柄。
type Runner interface {
	Start(context.Context, Spec) (Process, error)
	Read(id string, total, cursor int64, limit int) (Output, error)
	Remove(id string) error
}

// Store 保存任务状态，调用方串行提交同一任务的变更。
type Store interface {
	Load() ([]domain.Record, error)
	Save(domain.Record) error
	Delete(id string) error
}

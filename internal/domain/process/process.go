// Package process 定义命令任务及其归属和终态。
package process

import "time"

// Mode 区分有限任务与常驻服务。
type Mode string

const (
	ModeJob     Mode = "job"
	ModeService Mode = "service"
)

// State 是任务的执行状态。
type State string

const (
	Running     State = "running"
	Succeeded   State = "succeeded"
	Failed      State = "failed"
	Cancelled   State = "cancelled"
	TimedOut    State = "timed_out"
	Interrupted State = "interrupted"
)

// Owner 将任务限定到创建它的工作区和会话。
type Owner struct {
	Workspace string `json:"workspace"`
	SessionID string `json:"session_id"`
}

// Record 是可恢复的任务事实；恢复时不重新执行命令，也不接管旧 PID。
type Record struct {
	ID          string    `json:"id"`
	Owner       Owner     `json:"owner"`
	Command     string    `json:"command"`
	Mode        Mode      `json:"mode"`
	State       State     `json:"state"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	ExitCode    *int      `json:"exit_code,omitempty"`
	Error       string    `json:"error,omitempty"`
	OutputBytes int64     `json:"output_bytes"`
	Stdout      string    `json:"stdout,omitempty"`
	Stderr      string    `json:"stderr,omitempty"`
	Background  bool      `json:"background"`
	Delivered   bool      `json:"delivered"`
}

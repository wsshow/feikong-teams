// Package task 定义跨回合的用户任务，不与一次模型运行状态混用。
package task

import "time"

// Status 描述用户目标的进展。
type Status string

const (
	Pending     Status = "pending"
	InProgress  Status = "in_progress"
	WaitingUser Status = "waiting_user"
	WaitingTime Status = "waiting_time"
	Paused      Status = "paused"
	Completed   Status = "completed"
	Failed      Status = "failed"
	Cancelled   Status = "cancelled"
)

// Valid 判断是否属于任务状态。
func (s Status) Valid() bool {
	switch s {
	case Pending, InProgress, WaitingUser, WaitingTime, Paused, Completed, Failed, Cancelled:
		return true
	}
	return false
}

// Terminal 表示任务已经结束。
func (s Status) Terminal() bool { return s == Completed || s == Failed || s == Cancelled }

// Task 保存续办所需的最小上下文；Result 可包含已验证的交付物引用。
type Task struct {
	ID         string    `json:"id"`
	Key        string    `json:"key"`
	Version    uint64    `json:"version"`
	SessionID  string    `json:"session_id"`
	Title      string    `json:"title"`
	Goal       string    `json:"goal"`
	Acceptance string    `json:"acceptance"`
	Status     Status    `json:"status"`
	Progress   string    `json:"progress,omitempty"`
	NextStep   string    `json:"next_step,omitempty"`
	Result     string    `json:"result,omitempty"`
	ScheduleID string    `json:"schedule_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

package schedule

import "time"

const (
	MaxResultFileBytes      = 8 << 20
	MaxCollectedOutputBytes = 6 << 20
	MaxHistoryEntries       = 1_000
	MaxHistoryScanEntries   = 10_000
)

// Status 表示调度任务的生命周期状态。
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// ValidStatus 判断状态是否属于调度任务生命周期。
func ValidStatus(status Status) bool {
	switch status {
	case StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// Task 是调度任务的领域模型，只表达业务状态，不泄露文件路径等存储细节。
type Task struct {
	ID        string     `json:"id"`
	Task      string     `json:"task"`
	CronExpr  string     `json:"cron_expr,omitempty"`
	OneTime   bool       `json:"one_time"`
	NextRunAt time.Time  `json:"next_run_at"`
	Status    Status     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
}

// TaskList 是文件存储的结构化快照。
type TaskList struct {
	Tasks []Task `json:"tasks"`
}

// HistoryEntry 表示一次历史执行结果。
type HistoryEntry struct {
	Filename string `json:"filename"`
	Time     string `json:"time"`
}

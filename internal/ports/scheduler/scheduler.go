package scheduler

import (
	"context"

	domainschedule "fkteams/internal/domain/schedule"
)

// AddTaskRequest 描述创建调度任务所需的最小输入。
type AddTaskRequest struct {
	Task      string
	CronExpr  string
	ExecuteAt string
}

// TaskExecutor 执行已经到期的调度任务。
type TaskExecutor interface {
	Execute(ctx context.Context, taskID string, task string) (string, error)
}

// TaskService 是应用用例管理任务所需的最小能力。
type TaskService interface {
	AddTask(ctx context.Context, req AddTaskRequest) (*domainschedule.Task, error)
	UpdateTask(ctx context.Context, taskID string, req AddTaskRequest) (*domainschedule.Task, error)
	ListTasks(ctx context.Context, statusFilter domainschedule.Status) ([]domainschedule.Task, error)
	CancelTask(ctx context.Context, taskID string) error
	DeleteTask(ctx context.Context, taskID string) error
	ReadTaskResult(ctx context.Context, taskID string) (string, error)
	ListHistoryEntries(ctx context.Context, taskID string) ([]domainschedule.HistoryEntry, error)
	ReadHistoryFile(ctx context.Context, taskID string, filename string) (string, error)
}

// ResultStore 保存后台任务的一次执行结果与历史快照。
type ResultStore interface {
	SaveResult(taskID, task, result string) error
}

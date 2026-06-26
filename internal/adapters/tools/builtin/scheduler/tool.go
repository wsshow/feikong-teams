package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	appschedule "fkteams/internal/app/schedule"
	domainschedule "fkteams/internal/domain/schedule"
	runtimeport "fkteams/internal/ports/runtime"
	schedulerport "fkteams/internal/ports/scheduler"
)

// ServiceProvider 从工具执行上下文提供调度用例服务。
type ServiceProvider func(context.Context) *appschedule.Service

// Tools 是 schedule 工具适配器。
type Tools struct {
	service ServiceProvider
}

// NewTools 创建 schedule 工具适配器。
func NewTools(provider ServiceProvider) *Tools {
	if provider == nil {
		provider = appschedule.FromContext
	}
	return &Tools{service: provider}
}

func (t *Tools) serviceOrError(ctx context.Context) (*appschedule.Service, error) {
	if t == nil || t.service == nil {
		return nil, fmt.Errorf("scheduler service is not initialized")
	}
	service := t.service(ctx)
	if service == nil {
		return nil, fmt.Errorf("scheduler service is not initialized")
	}
	return service, nil
}

// GetTools 获取定时任务工具集合。
func (t *Tools) GetTools() ([]runtimeport.Tool, error) {
	var tools []runtimeport.Tool

	scheduleAddTool, err := runtimeport.InferTool("schedule_add",
		"创建定时任务，任务将由后台任务官（Tasker）独立执行，你不需要也无法参与执行。"+
			"支持两种模式：1) cron 表达式（重复任务），如 '*/5 * * * *' 每5分钟、'0 9 * * *' 每天9点；2) execute_at 指定时间（一次性任务）。"+
			"cron 表达式为标准5字段格式：分 时 日 月 周。创建成功后告知用户任务已交由后台调度器管理即可，不要承诺你会去执行。",
		t.ScheduleAdd)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleAddTool)

	scheduleListTool, err := runtimeport.InferTool("schedule_list",
		"列出所有定时任务，支持按状态过滤（pending/running/completed/failed/cancelled）。",
		t.ScheduleList)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleListTool)

	scheduleCancelTool, err := runtimeport.InferTool("schedule_cancel",
		"取消指定的定时任务，只能取消状态为 pending 的任务。",
		t.ScheduleCancel)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleCancelTool)

	scheduleDeleteTool, err := runtimeport.InferTool("schedule_delete",
		"永久删除指定的定时任务（从任务列表中移除）。不能删除正在执行中（running）的任务。",
		t.ScheduleDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleDeleteTool)

	return tools, nil
}

// ScheduleAddRequest 创建定时任务请求。
type ScheduleAddRequest struct {
	Task      string `json:"task" jsonschema:"description=任务描述，应清晰完整，足以让团队独立执行"`
	CronExpr  string `json:"cron_expr,omitempty" jsonschema:"description=标准 cron 表达式（5个字段：分 时 日 月 周），用于重复执行的定时任务。例如：*/5 * * * * 表示每5分钟，0 9 * * * 表示每天9点，0 9 * * 1-5 表示工作日9点"`
	ExecuteAt string `json:"execute_at,omitempty" jsonschema:"description=一次性任务的执行时间，格式为 ISO 8601（如 2025-01-15T09:00:00+08:00）。与 cron_expr 二选一"`
}

// ScheduleAddResponse 创建定时任务响应。
type ScheduleAddResponse struct {
	Success      bool                 `json:"success"`
	Message      string               `json:"message"`
	ErrorMessage string               `json:"error_message,omitempty"`
	Task         *domainschedule.Task `json:"task,omitempty"`
}

// ScheduleListRequest 列出定时任务请求。
type ScheduleListRequest struct {
	StatusFilter string `json:"status_filter,omitempty" jsonschema:"description=按状态过滤任务：pending/running/completed/failed/cancelled，留空则列出所有"`
}

// ScheduleListResponse 列出定时任务响应。
type ScheduleListResponse struct {
	Success      bool                  `json:"success"`
	TotalCount   int                   `json:"total_count"`
	ErrorMessage string                `json:"error_message,omitempty"`
	Tasks        []domainschedule.Task `json:"tasks,omitempty"`
}

// ScheduleCancelRequest 取消定时任务请求。
type ScheduleCancelRequest struct {
	TaskID string `json:"task_id" jsonschema:"description=要取消的任务 ID"`
}

// ScheduleCancelResponse 取消定时任务响应。
type ScheduleCancelResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ScheduleDeleteRequest 删除定时任务请求。
type ScheduleDeleteRequest struct {
	TaskID string `json:"task_id" jsonschema:"description=要删除的任务 ID"`
}

// ScheduleDeleteResponse 删除定时任务响应。
type ScheduleDeleteResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ScheduleAdd 添加定时任务。
func (t *Tools) ScheduleAdd(ctx context.Context, req *ScheduleAddRequest) (*ScheduleAddResponse, error) {
	service, err := t.serviceOrError(ctx)
	if err != nil {
		return &ScheduleAddResponse{ErrorMessage: err.Error()}, nil
	}
	task, err := service.AddTask(ctx, schedulerport.AddTaskRequest{
		Task:      req.Task,
		CronExpr:  req.CronExpr,
		ExecuteAt: req.ExecuteAt,
	})
	if err != nil {
		return &ScheduleAddResponse{ErrorMessage: err.Error()}, nil
	}
	return &ScheduleAddResponse{
		Success: true,
		Message: "task created, will be executed by background tasker on schedule",
		Task:    task,
	}, nil
}

// ScheduleList 列出定时任务。
func (t *Tools) ScheduleList(ctx context.Context, req *ScheduleListRequest) (*ScheduleListResponse, error) {
	service, err := t.serviceOrError(ctx)
	if err != nil {
		return &ScheduleListResponse{ErrorMessage: err.Error()}, nil
	}
	tasks, err := service.ListTasks(ctx, domainschedule.Status(req.StatusFilter))
	if err != nil {
		return &ScheduleListResponse{ErrorMessage: err.Error()}, nil
	}
	return &ScheduleListResponse{
		Success:    true,
		TotalCount: len(tasks),
		Tasks:      tasks,
	}, nil
}

// ScheduleCancel 取消定时任务。
func (t *Tools) ScheduleCancel(ctx context.Context, req *ScheduleCancelRequest) (*ScheduleCancelResponse, error) {
	service, err := t.serviceOrError(ctx)
	if err != nil {
		return &ScheduleCancelResponse{ErrorMessage: err.Error()}, nil
	}
	if err := service.CancelTask(ctx, req.TaskID); err != nil {
		return &ScheduleCancelResponse{ErrorMessage: err.Error()}, nil
	}
	return &ScheduleCancelResponse{Success: true, Message: "task cancelled"}, nil
}

// ScheduleDelete 删除定时任务。
func (t *Tools) ScheduleDelete(ctx context.Context, req *ScheduleDeleteRequest) (*ScheduleDeleteResponse, error) {
	service, err := t.serviceOrError(ctx)
	if err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: err.Error()}, nil
	}
	if err := service.DeleteTask(ctx, req.TaskID); err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: err.Error()}, nil
	}
	return &ScheduleDeleteResponse{Success: true, Message: "task deleted"}, nil
}

// FormatTasksForDisplay 格式化任务列表用于 CLI 显示。
func FormatTasksForDisplay(tasks []domainschedule.Task) string {
	if len(tasks) == 0 {
		return "no scheduled tasks"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d scheduled tasks:\n\n", len(tasks))

	for i, task := range tasks {
		statusIcon := "[pending]"
		switch task.Status {
		case domainschedule.StatusCompleted:
			statusIcon = "[done]"
		case domainschedule.StatusRunning:
			statusIcon = "[running]"
		case domainschedule.StatusFailed:
			statusIcon = "[failed]"
		case domainschedule.StatusCancelled:
			statusIcon = "[cancelled]"
		}

		fmt.Fprintf(&sb, "  %s %d. %s\n", statusIcon, i+1, task.Task)
		fmt.Fprintf(&sb, "     ID: %s | Status: %s\n", task.ID, task.Status)
		if task.CronExpr != "" {
			fmt.Fprintf(&sb, "     Cron: %s\n", task.CronExpr)
		}
		fmt.Fprintf(&sb, "     Next run: %s\n", task.NextRunAt.Format("2006-01-02 15:04:05"))
		if task.LastRunAt != nil {
			fmt.Fprintf(&sb, "     Last run: %s\n", task.LastRunAt.Format("2006-01-02 15:04:05"))
		}
		if i < len(tasks)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatTaskDetailJSON 格式化单个任务为 JSON 字符串。
func FormatTaskDetailJSON(task domainschedule.Task) string {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Sprintf("marshal failed: %v", err)
	}
	return string(data)
}

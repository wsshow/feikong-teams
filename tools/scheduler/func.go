package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ScheduleAddRequest 创建定时任务请求
type ScheduleAddRequest struct {
	Task      string `json:"task" jsonschema:"description=任务描述，应清晰完整，足以让团队独立执行"`
	CronExpr  string `json:"cron_expr,omitempty" jsonschema:"description=标准 cron 表达式（5个字段：分 时 日 月 周），用于重复执行的定时任务。例如：*/5 * * * * 表示每5分钟，0 9 * * * 表示每天9点，0 9 * * 1-5 表示工作日9点"`
	ExecuteAt string `json:"execute_at,omitempty" jsonschema:"description=一次性任务的执行时间，格式为 ISO 8601（如 2025-01-15T09:00:00+08:00）。与 cron_expr 二选一"`
}

// ScheduleAddResponse 创建定时任务响应
type ScheduleAddResponse struct {
	Success      bool           `json:"success"`
	Message      string         `json:"message"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Task         *ScheduledTask `json:"task,omitempty"`
}

// ScheduleListRequest 列出定时任务请求
type ScheduleListRequest struct {
	StatusFilter string `json:"status_filter,omitempty" jsonschema:"description=按状态过滤任务：pending/running/completed/failed/cancelled，留空则列出所有"`
}

// ScheduleListResponse 列出定时任务响应
type ScheduleListResponse struct {
	Success      bool            `json:"success"`
	TotalCount   int             `json:"total_count"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Tasks        []ScheduledTask `json:"tasks,omitempty"`
}

// ScheduleCancelRequest 取消定时任务请求
type ScheduleCancelRequest struct {
	TaskID string `json:"task_id" jsonschema:"description=要取消的任务 ID"`
}

// ScheduleCancelResponse 取消定时任务响应
type ScheduleCancelResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ScheduleAdd 添加定时任务
func (s *Scheduler) ScheduleAdd(ctx context.Context, req *ScheduleAddRequest) (*ScheduleAddResponse, error) {
	if req.Task == "" {
		return &ScheduleAddResponse{ErrorMessage: "任务描述不能为空"}, nil
	}

	if req.CronExpr == "" && req.ExecuteAt == "" {
		return &ScheduleAddResponse{ErrorMessage: "必须提供 cron_expr（重复任务）或 execute_at（一次性任务）"}, nil
	}

	if req.CronExpr != "" && req.ExecuteAt != "" {
		return &ScheduleAddResponse{ErrorMessage: "cron_expr 和 execute_at 不能同时指定"}, nil
	}

	task := ScheduledTask{
		ID:        fmt.Sprintf("sched_%d", time.Now().UnixNano()),
		Task:      req.Task,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	if req.CronExpr != "" {
		// 重复任务
		expr := strings.TrimSpace(req.CronExpr)
		nextRun, err := s.ParseCronExpr(expr)
		if err != nil {
			return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("无效的 cron 表达式: %v", err)}, nil
		}
		task.CronExpr = expr
		task.OneTime = false
		task.NextRunAt = nextRun
	} else {
		// 一次性任务
		executeAt, err := time.Parse(time.RFC3339, req.ExecuteAt)
		if err != nil {
			return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("无效的时间格式，请使用 ISO 8601 格式: %v", err)}, nil
		}
		if executeAt.Before(time.Now()) {
			return &ScheduleAddResponse{ErrorMessage: "执行时间不能是过去的时间"}, nil
		}
		task.OneTime = true
		task.NextRunAt = executeAt
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("加载任务列表失败: %v", err)}, nil
	}

	tasks.Tasks = append(tasks.Tasks, task)
	if err := s.saveTasks(tasks); err != nil {
		return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("保存任务列表失败: %v", err)}, nil
	}

	return &ScheduleAddResponse{
		Success: true,
		Message: "定时任务创建成功",
		Task:    &task,
	}, nil
}

// ScheduleList 列出定时任务
func (s *Scheduler) ScheduleList(ctx context.Context, req *ScheduleListRequest) (*ScheduleListResponse, error) {
	tasks, err := s.GetTasks(req.StatusFilter)
	if err != nil {
		return &ScheduleListResponse{ErrorMessage: fmt.Sprintf("获取任务列表失败: %v", err)}, nil
	}

	return &ScheduleListResponse{
		Success:    true,
		TotalCount: len(tasks),
		Tasks:      tasks,
	}, nil
}

// ScheduleCancel 取消定时任务
func (s *Scheduler) ScheduleCancel(ctx context.Context, req *ScheduleCancelRequest) (*ScheduleCancelResponse, error) {
	if req.TaskID == "" {
		return &ScheduleCancelResponse{ErrorMessage: "任务 ID 不能为空"}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return &ScheduleCancelResponse{ErrorMessage: fmt.Sprintf("加载任务列表失败: %v", err)}, nil
	}

	found := false
	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID == req.TaskID {
			if tasks.Tasks[i].Status != "pending" {
				return &ScheduleCancelResponse{ErrorMessage: fmt.Sprintf("任务状态为 %s，只能取消 pending 状态的任务", tasks.Tasks[i].Status)}, nil
			}
			tasks.Tasks[i].Status = "cancelled"
			found = true
			break
		}
	}

	if !found {
		return &ScheduleCancelResponse{ErrorMessage: "未找到指定的任务"}, nil
	}

	if err := s.saveTasks(tasks); err != nil {
		return &ScheduleCancelResponse{ErrorMessage: fmt.Sprintf("保存任务列表失败: %v", err)}, nil
	}

	return &ScheduleCancelResponse{
		Success: true,
		Message: "任务已取消",
	}, nil
}

// ScheduleDeleteRequest 删除定时任务请求
type ScheduleDeleteRequest struct {
	TaskID string `json:"task_id" jsonschema:"description=要删除的任务 ID"`
}

// ScheduleDeleteResponse 删除定时任务响应
type ScheduleDeleteResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ScheduleDelete 删除定时任务（从列表中永久移除）
func (s *Scheduler) ScheduleDelete(ctx context.Context, req *ScheduleDeleteRequest) (*ScheduleDeleteResponse, error) {
	if req.TaskID == "" {
		return &ScheduleDeleteResponse{ErrorMessage: "任务 ID 不能为空"}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: fmt.Sprintf("加载任务列表失败: %v", err)}, nil
	}

	found := false
	var remaining []ScheduledTask
	for _, t := range tasks.Tasks {
		if t.ID == req.TaskID {
			if t.Status == "running" {
				return &ScheduleDeleteResponse{ErrorMessage: "不能删除正在执行中的任务，请先取消"}, nil
			}
			found = true
			continue
		}
		remaining = append(remaining, t)
	}

	if !found {
		return &ScheduleDeleteResponse{ErrorMessage: "未找到指定的任务"}, nil
	}

	tasks.Tasks = remaining
	if err := s.saveTasks(tasks); err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: fmt.Sprintf("保存任务列表失败: %v", err)}, nil
	}

	return &ScheduleDeleteResponse{
		Success: true,
		Message: "任务已删除",
	}, nil
}

// FormatTasksForDisplay 格式化任务列表用于 CLI 显示
func FormatTasksForDisplay(tasks []ScheduledTask) string {
	if len(tasks) == 0 {
		return "暂无定时任务"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个定时任务:\n\n", len(tasks)))

	for i, t := range tasks {
		statusIcon := "[等待]"
		switch t.Status {
		case "completed":
			statusIcon = "[完成]"
		case "running":
			statusIcon = "[运行]"
		case "failed":
			statusIcon = "[失败]"
		case "cancelled":
			statusIcon = "[取消]"
		}

		sb.WriteString(fmt.Sprintf("  %s %d. %s\n", statusIcon, i+1, t.Task))
		sb.WriteString(fmt.Sprintf("     ID: %s | 状态: %s\n", t.ID, t.Status))

		if t.CronExpr != "" {
			sb.WriteString(fmt.Sprintf("     Cron: %s\n", t.CronExpr))
		}

		sb.WriteString(fmt.Sprintf("     下次执行: %s\n", t.NextRunAt.Format("2006-01-02 15:04:05")))

		if t.LastRunAt != nil {
			sb.WriteString(fmt.Sprintf("     上次执行: %s\n", t.LastRunAt.Format("2006-01-02 15:04:05")))
		}

		if t.Result != "" {
			result := t.Result
			if len(result) > 100 {
				result = result[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("     结果: %s\n", result))
		}

		if i < len(tasks)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatTaskDetailJSON 格式化单个任务为 JSON 字符串（供终端命令查看详情）
func FormatTaskDetailJSON(task ScheduledTask) string {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Sprintf("格式化失败: %v", err)
	}
	return string(data)
}

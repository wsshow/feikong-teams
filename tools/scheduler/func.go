package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		return &ScheduleAddResponse{ErrorMessage: "task description is required"}, nil
	}

	if req.CronExpr == "" && req.ExecuteAt == "" {
		return &ScheduleAddResponse{ErrorMessage: "must provide cron_expr (recurring) or execute_at (one-time)"}, nil
	}

	if req.CronExpr != "" && req.ExecuteAt != "" {
		return &ScheduleAddResponse{ErrorMessage: "cron_expr and execute_at are mutually exclusive"}, nil
	}

	task := ScheduledTask{
		ID:        generateTaskID(),
		Task:      req.Task,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	if req.CronExpr != "" {
		expr := strings.TrimSpace(req.CronExpr)
		nextRun, err := s.ParseCronExpr(expr)
		if err != nil {
			return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("invalid cron expression: %v", err)}, nil
		}
		task.CronExpr = expr
		task.OneTime = false
		task.NextRunAt = nextRun
	} else {
		executeAt, err := time.Parse(time.RFC3339, req.ExecuteAt)
		if err != nil {
			return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("invalid time format, use ISO 8601: %v", err)}, nil
		}
		if executeAt.Before(time.Now()) {
			return &ScheduleAddResponse{ErrorMessage: "execute_at must be in the future"}, nil
		}
		task.OneTime = true
		task.NextRunAt = executeAt
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("load task list failed: %v", err)}, nil
	}

	tasks.Tasks = append(tasks.Tasks, task)
	if err := s.saveTasks(tasks); err != nil {
		return &ScheduleAddResponse{ErrorMessage: fmt.Sprintf("save task list failed: %v", err)}, nil
	}

	return &ScheduleAddResponse{
		Success: true,
		Message: "task created, will be executed by background tasker on schedule",
		Task:    &task,
	}, nil
}

// ScheduleList 列出定时任务
func (s *Scheduler) ScheduleList(ctx context.Context, req *ScheduleListRequest) (*ScheduleListResponse, error) {
	tasks, err := s.GetTasks(req.StatusFilter)
	if err != nil {
		return &ScheduleListResponse{ErrorMessage: fmt.Sprintf("get task list failed: %v", err)}, nil
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
		return &ScheduleCancelResponse{ErrorMessage: "task ID is required"}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return &ScheduleCancelResponse{ErrorMessage: fmt.Sprintf("load task list failed: %v", err)}, nil
	}

	found := false
	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID == req.TaskID {
			if tasks.Tasks[i].Status != "pending" {
				return &ScheduleCancelResponse{ErrorMessage: fmt.Sprintf("task status is %s, only pending tasks can be cancelled", tasks.Tasks[i].Status)}, nil
			}
			tasks.Tasks[i].Status = "cancelled"
			found = true
			break
		}
	}

	if !found {
		return &ScheduleCancelResponse{ErrorMessage: "task not found"}, nil
	}

	if err := s.saveTasks(tasks); err != nil {
		return &ScheduleCancelResponse{ErrorMessage: fmt.Sprintf("save task list failed: %v", err)}, nil
	}

	return &ScheduleCancelResponse{
		Success: true,
		Message: "task cancelled",
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
		return &ScheduleDeleteResponse{ErrorMessage: "task ID is required"}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: fmt.Sprintf("load task list failed: %v", err)}, nil
	}

	found := false
	var remaining []ScheduledTask
	for _, t := range tasks.Tasks {
		if t.ID == req.TaskID {
			if t.Status == "running" {
				return &ScheduleDeleteResponse{ErrorMessage: "cannot delete a running task, cancel it first"}, nil
			}
			found = true
			continue
		}
		remaining = append(remaining, t)
	}

	if !found {
		return &ScheduleDeleteResponse{ErrorMessage: "task not found"}, nil
	}

	// remove per-task result directory
	if err := os.RemoveAll(s.taskDir(req.TaskID)); err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: fmt.Sprintf("remove task dir failed: %v", err)}, nil
	}

	tasks.Tasks = remaining
	if err := s.saveTasks(tasks); err != nil {
		return &ScheduleDeleteResponse{ErrorMessage: fmt.Sprintf("save task list failed: %v", err)}, nil
	}

	return &ScheduleDeleteResponse{
		Success: true,
		Message: "task deleted",
	}, nil
}

// FormatTasksForDisplay 格式化任务列表用于 CLI 显示
func FormatTasksForDisplay(tasks []ScheduledTask) string {
	if len(tasks) == 0 {
		return "no scheduled tasks"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d scheduled tasks:\n\n", len(tasks))

	for i, t := range tasks {
		statusIcon := "[pending]"
		switch t.Status {
		case "completed":
			statusIcon = "[done]"
		case "running":
			statusIcon = "[running]"
		case "failed":
			statusIcon = "[failed]"
		case "cancelled":
			statusIcon = "[cancelled]"
		}

		fmt.Fprintf(&sb, "  %s %d. %s\n", statusIcon, i+1, t.Task)
		fmt.Fprintf(&sb, "     ID: %s | Status: %s\n", t.ID, t.Status)

		if t.CronExpr != "" {
			fmt.Fprintf(&sb, "     Cron: %s\n", t.CronExpr)
		}

		fmt.Fprintf(&sb, "     Next run: %s\n", t.NextRunAt.Format("2006-01-02 15:04:05"))

		if t.LastRunAt != nil {
			fmt.Fprintf(&sb, "     Last run: %s\n", t.LastRunAt.Format("2006-01-02 15:04:05"))
		}

		if t.ResultPath != "" {
			fmt.Fprintf(&sb, "     Result: %s\n", t.ResultPath)
			historyDir := filepath.Join(filepath.Dir(t.ResultPath), "history")
			if entries, err := os.ReadDir(historyDir); err == nil && len(entries) > 0 {
				fmt.Fprintf(&sb, "     History: %d past results\n", len(entries))
			}
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
		return fmt.Sprintf("marshal failed: %v", err)
	}
	return string(data)
}

// HistoryEntry represents a single history result file
type HistoryEntry struct {
	Filename string `json:"filename"`
	Time     string `json:"time"`
}

// ListHistoryEntries lists all history result files for a task
func (s *Scheduler) ListHistoryEntries(taskID string) ([]HistoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	historyDir := filepath.Join(s.taskDir(taskID), "history")
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	var result []HistoryEntry
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		name := e.Name()
		// format: 20260430_150405.md → 2026-04-30 15:04:05
		timeStr := ""
		if len(name) >= 15 {
			timeStr = fmt.Sprintf("%s-%s-%s %s:%s:%s",
				name[0:4], name[4:6], name[6:8],
				name[9:11], name[11:13], name[13:15])
		}
		result = append(result, HistoryEntry{
			Filename: name,
			Time:     timeStr,
		})
	}

	return result, nil
}

// ReadHistoryFile reads a specific history file for a task
func (s *Scheduler) ReadHistoryFile(taskID string, filename string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// prevent path traversal
	filename = filepath.Base(filename)
	if filepath.Ext(filename) != ".md" {
		return "", fmt.Errorf("invalid file type")
	}

	filePath := filepath.Join(s.taskDir(taskID), "history", filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read history file: %w", err)
	}
	return string(data), nil
}

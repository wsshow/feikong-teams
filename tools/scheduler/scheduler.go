package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ScheduledTask 定时任务
type ScheduledTask struct {
	ID        string     `json:"id"`
	Task      string     `json:"task"`                // 任务描述（发送给团队执行的查询）
	CronExpr  string     `json:"cron_expr,omitempty"` // cron 表达式（重复任务）
	OneTime   bool       `json:"one_time"`            // 是否一次性任务
	NextRunAt time.Time  `json:"next_run_at"`         // 下次执行时间
	Status    string     `json:"status"`              // pending, running, completed, failed, cancelled
	CreatedAt time.Time  `json:"created_at"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	Result    string     `json:"result,omitempty"`
}

// ScheduledTaskList 定时任务列表
type ScheduledTaskList struct {
	Tasks []ScheduledTask `json:"tasks"`
}

// Scheduler 定时任务调度器
type Scheduler struct {
	filePath   string
	mu         sync.RWMutex
	stopCh     chan struct{}
	executor   TaskExecutor
	running    bool
	cronParser cron.Parser
}

// TaskExecutor 任务执行器接口
type TaskExecutor interface {
	Execute(task string) (string, error)
}

var (
	globalScheduler *Scheduler
	schedulerMu     sync.Mutex
)

// InitGlobal 初始化全局调度器（幂等，失败后可重试）
func InitGlobal(baseDir string) (*Scheduler, error) {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()

	if globalScheduler != nil {
		return globalScheduler, nil
	}

	s, err := newScheduler(baseDir)
	if err != nil {
		return nil, err
	}
	globalScheduler = s
	return globalScheduler, nil
}

// Global 获取全局调度器
func Global() *Scheduler {
	return globalScheduler
}

func newScheduler(baseDir string) (*Scheduler, error) {
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create scheduler directory: %w", err)
	}

	filePath := filepath.Join(absPath, "scheduled_tasks.json")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		emptyList := ScheduledTaskList{Tasks: []ScheduledTask{}}
		data, err := json.MarshalIndent(emptyList, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal task list: %w", err)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to create task list file: %w", err)
		}
	}

	return &Scheduler{
		filePath:   filePath,
		stopCh:     make(chan struct{}),
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}, nil
}

// ParseCronExpr 解析 cron 表达式并返回下次执行时间
func (s *Scheduler) ParseCronExpr(expr string) (time.Time, error) {
	sched, err := s.cronParser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return sched.Next(time.Now()), nil
}

// ComputeNextRun 基于 cron 表达式计算指定时间之后的下次执行时间
func (s *Scheduler) ComputeNextRun(expr string, after time.Time) (time.Time, error) {
	sched, err := s.cronParser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	return sched.Next(after), nil
}

// SetExecutor 设置任务执行器
func (s *Scheduler) SetExecutor(executor TaskExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
}

// Start 启动调度器后台轮询
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// 恢复上次中断遗留的 running 状态任务
	s.recoverStaleRunningTasks()

	go s.run()
}

// recoverStaleRunningTasks 将上次中断遗留的 running 状态任务恢复为 pending
func (s *Scheduler) recoverStaleRunningTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return
	}

	changed := false
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Status == "running" {
			tasks.Tasks[i].Status = "pending"
			changed = true
		}
	}

	if changed {
		_ = s.saveTasks(tasks)
	}
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 启动时立即检查一次
	s.checkAndExecute()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkAndExecute()
		}
	}
}

func (s *Scheduler) checkAndExecute() {
	s.mu.RLock()
	executor := s.executor
	s.mu.RUnlock()

	if executor == nil {
		return
	}

	tasks, err := s.loadTasks()
	if err != nil {
		return
	}

	now := time.Now()
	for i := range tasks.Tasks {
		task := &tasks.Tasks[i]
		if task.Status != "pending" {
			continue
		}
		if now.Before(task.NextRunAt) {
			continue
		}

		// 到达执行时间
		task.Status = "running"
		task.LastRunAt = &now
		_ = s.saveTasks(tasks)

		go s.executeTask(task.ID, task.Task, task.CronExpr, task.OneTime, executor)
	}
}

func (s *Scheduler) executeTask(taskID string, taskContent string, cronExpr string, oneTime bool, executor TaskExecutor) {
	result, err := executor.Execute(taskContent)

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, loadErr := s.loadTasks()
	if loadErr != nil {
		return
	}

	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID == taskID {
			now := time.Now()
			tasks.Tasks[i].LastRunAt = &now
			if err != nil {
				tasks.Tasks[i].Status = "failed"
				tasks.Tasks[i].Result = fmt.Sprintf("执行失败: %v", err)
			} else {
				tasks.Tasks[i].Result = result
				if oneTime {
					tasks.Tasks[i].Status = "completed"
				} else {
					// 重复任务：计算下次执行时间
					nextRun, cronErr := s.ComputeNextRun(cronExpr, now)
					if cronErr != nil {
						tasks.Tasks[i].Status = "failed"
						tasks.Tasks[i].Result = fmt.Sprintf("cron expression error: %v", cronErr)
					} else {
						tasks.Tasks[i].Status = "pending"
						tasks.Tasks[i].NextRunAt = nextRun
					}
				}
			}
			break
		}
	}

	_ = s.saveTasks(tasks)
}

// GetTasks 获取任务列表（供外部查询）
func (s *Scheduler) GetTasks(statusFilter string) ([]ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return nil, err
	}

	if statusFilter == "" {
		return tasks.Tasks, nil
	}

	var filtered []ScheduledTask
	for _, t := range tasks.Tasks {
		if t.Status == statusFilter {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func (s *Scheduler) loadTasks() (*ScheduledTaskList, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task list: %w", err)
	}

	var list ScheduledTaskList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to parse task list: %w", err)
	}

	return &list, nil
}

func (s *Scheduler) saveTasks(list *ScheduledTaskList) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task list: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to save task list: %w", err)
	}

	return nil
}

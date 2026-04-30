package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fkteams/log"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// ScheduledTask 定时任务
type ScheduledTask struct {
	ID         string     `json:"id"`
	Task       string     `json:"task"`
	CronExpr   string     `json:"cron_expr,omitempty"`
	OneTime    bool       `json:"one_time"`
	NextRunAt  time.Time  `json:"next_run_at"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	ResultPath string     `json:"result_path,omitempty"`
}

// ScheduledTaskList 定时任务列表
type ScheduledTaskList struct {
	Tasks []ScheduledTask `json:"tasks"`
}

// Scheduler 定时任务调度器
type Scheduler struct {
	filePath    string
	resultsDir  string
	mu          sync.RWMutex
	stopCh      chan struct{}
	executor    TaskExecutor
	running     bool
	cronParser  cron.Parser
	semaphore   chan struct{}
	cancelFuncs map[string]context.CancelFunc
	cancelsMu   sync.Mutex
}

// TaskExecutor 任务执行器接口
type TaskExecutor interface {
	Execute(ctx context.Context, taskID string, task string) (string, error)
}

const (
	maxConcurrentTasks = 5
	taskResultTTL      = 7 * 24 * time.Hour
)

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

	resultsDir := filepath.Join(absPath, "tasks")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create tasks directory: %w", err)
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
		filePath:    filePath,
		resultsDir:  resultsDir,
		stopCh:      make(chan struct{}),
		cronParser:  cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		semaphore:   make(chan struct{}, maxConcurrentTasks),
		cancelFuncs: make(map[string]context.CancelFunc),
	}, nil
}

// generateTaskID 生成基于 UUID v4 的任务 ID
func generateTaskID() string {
	return uuid.New().String()
}

// taskDir 返回任务的结果存储目录
func (s *Scheduler) taskDir(taskID string) string {
	return filepath.Join(s.resultsDir, taskID)
}

// taskResultPath 返回任务结果文件路径
func (s *Scheduler) taskResultPath(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "result.md")
}

// ensureTaskDir 确保任务目录存在
func (s *Scheduler) ensureTaskDir(taskID string) error {
	return os.MkdirAll(s.taskDir(taskID), 0755)
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

	s.recoverStaleRunningTasks()
	go s.run()
}

// recoverStaleRunningTasks 将上次中断遗留的 running 状态任务恢复为 pending
func (s *Scheduler) recoverStaleRunningTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		log.Printf("[scheduler] recover stale tasks failed: %v", err)
		return
	}

	changed := false
	for i := range tasks.Tasks {
		if tasks.Tasks[i].Status == "running" {
			tasks.Tasks[i].Status = "pending"
			changed = true
			log.Printf("[scheduler] recover stale task: %s → pending", tasks.Tasks[i].ID)
		}
	}

	if changed {
		if err := s.saveTasks(tasks); err != nil {
			log.Printf("[scheduler] save recovered tasks failed: %v", err)
		}
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

	// 取消所有正在执行的任务
	s.cancelsMu.Lock()
	for taskID, cancel := range s.cancelFuncs {
		log.Printf("[scheduler] cancelling running task: %s", taskID)
		cancel()
	}
	s.cancelsMu.Unlock()
}

func (s *Scheduler) run() {
	// 启动时立即检查一次
	s.checkAndExecute()

	for {
		// 计算下次检查间隔
		waitDuration := s.nextCheckDuration()

		timer := time.NewTimer(waitDuration)
		select {
		case <-s.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			s.checkAndExecute()
		}
	}
}

// nextCheckDuration 计算距下次任务到期的等待时间
func (s *Scheduler) nextCheckDuration() time.Duration {
	tasks, err := s.loadTasks()
	if err != nil {
		return 30 * time.Second
	}

	now := time.Now()
	minWait := 30 * time.Second
	for _, t := range tasks.Tasks {
		if t.Status != "pending" {
			continue
		}
		wait := t.NextRunAt.Sub(now)
		if wait <= 0 {
			return 0
		}
		if wait < minWait {
			minWait = wait + 500*time.Millisecond
		}
	}

	// 至少每 30 秒检查一次，用于 TTL 清理
	if minWait > 30*time.Second {
		minWait = 30 * time.Second
	}

	return minWait
}

func (s *Scheduler) checkAndExecute() {
	// TTL 清理（每轮检查时顺便执行）
	s.cleanupExpiredTasks()

	s.mu.RLock()
	executor := s.executor
	s.mu.RUnlock()

	if executor == nil {
		return
	}

	tasks, err := s.loadTasks()
	if err != nil {
		log.Printf("[scheduler] load tasks failed: %v", err)
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

		// 加写锁二次确认状态，防止重复执行
		s.mu.Lock()
		currentTasks, loadErr := s.loadTasks()
		if loadErr != nil {
			s.mu.Unlock()
			log.Printf("[scheduler] re-check load failed: %v", loadErr)
			continue
		}

		var currentTask *ScheduledTask
		for j := range currentTasks.Tasks {
			if currentTasks.Tasks[j].ID == task.ID {
				currentTask = &currentTasks.Tasks[j]
				break
			}
		}

		if currentTask == nil || currentTask.Status != "pending" {
			s.mu.Unlock()
			continue
		}

		currentTask.Status = "running"
		currentTask.LastRunAt = &now
		if saveErr := s.saveTasks(currentTasks); saveErr != nil {
			s.mu.Unlock()
			log.Printf("[scheduler] save task status failed: %v", saveErr)
			continue
		}
		s.mu.Unlock()

		// 通过 semaphore 控制并发
		select {
		case s.semaphore <- struct{}{}:
			go func(tID, tContent, tCron string, tOneTime bool, tExec TaskExecutor) {
				defer func() { <-s.semaphore }()
				s.executeTask(tID, tContent, tCron, tOneTime, tExec)
			}(currentTask.ID, currentTask.Task, currentTask.CronExpr, currentTask.OneTime, executor)
		default:
			// 并发已满，回退状态到 pending
			log.Printf("[scheduler] max concurrent reached (%d), task %s deferred", maxConcurrentTasks, currentTask.ID)
			s.mu.Lock()
			fallbackTasks, _ := s.loadTasks()
			if fallbackTasks != nil {
				for j := range fallbackTasks.Tasks {
					if fallbackTasks.Tasks[j].ID == currentTask.ID {
						fallbackTasks.Tasks[j].Status = "pending"
						_ = s.saveTasks(fallbackTasks)
						break
					}
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Scheduler) executeTask(taskID string, taskContent string, cronExpr string, oneTime bool, executor TaskExecutor) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	s.cancelsMu.Lock()
	s.cancelFuncs[taskID] = cancel
	s.cancelsMu.Unlock()

	defer func() {
		s.cancelsMu.Lock()
		delete(s.cancelFuncs, taskID)
		s.cancelsMu.Unlock()
	}()

	log.Printf("[scheduler] task started: %s", taskID)
	_, err := executor.Execute(ctx, taskID, taskContent)

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, loadErr := s.loadTasks()
	if loadErr != nil {
		log.Printf("[scheduler] load after execute failed (result lost): taskID=%s, err=%v", taskID, loadErr)
		return
	}

	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID == taskID {
			now := time.Now()
			tasks.Tasks[i].LastRunAt = &now

			tasks.Tasks[i].ResultPath = s.taskResultPath(taskID)
			if err != nil {
				tasks.Tasks[i].Status = "failed"
				log.Printf("[scheduler] task failed: %s, err=%v", taskID, err)
			} else {
				if oneTime {
					tasks.Tasks[i].Status = "completed"
				} else {
					nextRun, cronErr := s.ComputeNextRun(cronExpr, now)
					if cronErr != nil {
						tasks.Tasks[i].Status = "failed"
						log.Printf("[scheduler] cron parse failed: taskID=%s, err=%v", taskID, cronErr)
					} else {
						tasks.Tasks[i].Status = "pending"
						tasks.Tasks[i].NextRunAt = nextRun
					}
				}
			}
			log.Printf("[scheduler] task done: %s, status=%s", taskID, tasks.Tasks[i].Status)
			break
		}
	}

	if saveErr := s.saveTasks(tasks); saveErr != nil {
		log.Printf("[scheduler] save result failed: taskID=%s, err=%v", taskID, saveErr)
	}
}

// cleanupExpiredTasks 清理超过 TTL 的已完成/失败/取消的一次性任务
func (s *Scheduler) cleanupExpiredTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-taskResultTTL)
	var remaining []ScheduledTask
	removed := 0

	for _, t := range tasks.Tasks {
		if t.Status == "completed" || t.Status == "failed" || t.Status == "cancelled" {
			refTime := t.LastRunAt
			if refTime == nil {
				refTime = &t.CreatedAt
			}
			if refTime.Before(cutoff) {
				// 删除任务目录
				if err := os.RemoveAll(s.taskDir(t.ID)); err != nil {
					log.Printf("[scheduler] cleanup task dir failed: taskID=%s, err=%v", t.ID, err)
				}
				removed++
				continue
			}
		}
		remaining = append(remaining, t)
	}

	if removed > 0 {
		log.Printf("[scheduler] cleaned up %d expired tasks", removed)
		tasks.Tasks = remaining
		if err := s.saveTasks(tasks); err != nil {
			log.Printf("[scheduler] save after cleanup failed: %v", err)
		}
	}
}

// CancelExecution 取消正在执行的任务
func (s *Scheduler) CancelExecution(taskID string) {
	s.cancelsMu.Lock()
	if cancel, ok := s.cancelFuncs[taskID]; ok {
		cancel()
		delete(s.cancelFuncs, taskID)
		log.Printf("[scheduler] task execution cancelled: %s", taskID)
	}
	s.cancelsMu.Unlock()
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

// loadTaskByID 在持有锁的情况下根据 ID 查找任务
func (s *Scheduler) loadTaskByID(taskID string) (*ScheduledTask, error) {
	tasks, err := s.loadTasks()
	if err != nil {
		return nil, err
	}
	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID == taskID {
			return &tasks.Tasks[i], nil
		}
	}
	return nil, nil
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

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// ReadTaskResult reads the execution result for a task
func (s *Scheduler) ReadTaskResult(taskID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, err := s.loadTaskByID(taskID)
	if err != nil {
		return "", fmt.Errorf("load task: %w", err)
	}
	if task == nil {
		return "", fmt.Errorf("task not found: %s", taskID)
	}
	if task.ResultPath == "" {
		return "", fmt.Errorf("task %s has no result yet", taskID)
	}

	data, err := os.ReadFile(task.ResultPath)
	if err != nil {
		return "", fmt.Errorf("read result file: %w", err)
	}
	return string(data), nil
}

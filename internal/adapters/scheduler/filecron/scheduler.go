package filecron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fkteams/internal/domain/apperror"
	domainschedule "fkteams/internal/domain/schedule"
	schedulerport "fkteams/internal/ports/scheduler"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/log"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// Scheduler 定时任务调度器
type Scheduler struct {
	filePath    string
	resultsDir  string
	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	stopCh      chan struct{}
	runCtx      context.Context
	runCancel   context.CancelFunc
	stopDone    chan struct{}
	wg          sync.WaitGroup
	executor    schedulerport.TaskExecutor
	running     bool
	stopping    bool
	cronParser  cron.Parser
	semaphore   chan struct{}
	cancelFuncs map[string]context.CancelFunc
	cancelsMu   sync.Mutex
}

const (
	maxConcurrentTasks            = 5
	maxScheduledTasks             = 1_000
	maxTaskDescriptionBytes       = 16 << 10
	maxTaskStoreBytes       int64 = 32 << 20
	maxResultDirectories          = 10_000
	taskResultTTL                 = 7 * 24 * time.Hour
)

// NewScheduler 创建基于文件存储和 cron 计算的调度器。
func NewScheduler(baseDir string) (*Scheduler, error) {
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

	info, statErr := os.Stat(filePath)
	if os.IsNotExist(statErr) {
		emptyList := domainschedule.TaskList{Tasks: []domainschedule.Task{}}
		data, err := json.MarshalIndent(emptyList, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal task list: %w", err)
		}
		if err := atomicfile.WriteFile(filePath, data, 0644); err != nil {
			return nil, fmt.Errorf("failed to create task list file: %w", err)
		}
	} else if statErr != nil {
		return nil, fmt.Errorf("failed to inspect task list file: %w", statErr)
	} else if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("task list path is not a regular file")
	}

	scheduler := &Scheduler{
		filePath:    filePath,
		resultsDir:  resultsDir,
		stopCh:      make(chan struct{}),
		cronParser:  cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
		semaphore:   make(chan struct{}, maxConcurrentTasks),
		cancelFuncs: make(map[string]context.CancelFunc),
	}
	tasks, err := scheduler.loadTasks()
	if err != nil {
		return nil, fmt.Errorf("load scheduler task list: %w", err)
	}
	scheduler.cleanupOrphanTaskDirs(tasks)
	return scheduler, nil
}

// generateTaskID 生成基于 UUID v4 的任务 ID
func generateTaskID() string {
	return uuid.New().String()
}

func validTaskID(taskID string) bool {
	return taskID != "" && taskID != "." && taskID != ".." && len(taskID) <= 200 &&
		filepath.Base(taskID) == taskID && !strings.Contains(taskID, `\`)
}

// taskDir 返回任务的结果存储目录
func (s *Scheduler) taskDir(taskID string) string {
	return filepath.Join(s.resultsDir, taskID)
}

// taskResultPath 返回任务结果文件路径
func (s *Scheduler) taskResultPath(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "result.md")
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

// AddTask 创建调度任务。
func (s *Scheduler) AddTask(ctx context.Context, req schedulerport.AddTaskRequest) (*domainschedule.Task, error) {
	task := domainschedule.Task{
		ID:        generateTaskID(),
		CreatedAt: time.Now(),
		Status:    domainschedule.StatusPending,
	}
	if err := s.applyTaskSchedule(&task, req); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	if len(tasks.Tasks) >= maxScheduledTasks {
		return nil, apperror.New(apperror.CodeResourceLimit, "scheduled task limit reached")
	}
	tasks.Tasks = append(tasks.Tasks, task)
	if err := s.saveTasks(tasks); err != nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	return &task, nil
}

// UpdateTask 更新非运行中的调度任务，并重新计算下次执行时间。
func (s *Scheduler) UpdateTask(ctx context.Context, taskID string, req schedulerport.AddTaskRequest) (*domainschedule.Task, error) {
	if !validTaskID(taskID) {
		return nil, apperror.New(apperror.CodeInvalidArgument, "invalid task ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID != taskID {
			continue
		}
		if tasks.Tasks[i].Status == domainschedule.StatusRunning {
			return nil, apperror.New(apperror.CodeConflict, "cannot update a running task")
		}
		if s.isExecuting(taskID) {
			return nil, apperror.New(apperror.CodeConflict, "cannot update a task while its execution is stopping")
		}
		next := tasks.Tasks[i]
		next.Status = domainschedule.StatusPending
		next.LastRunAt = nil
		if err := s.applyTaskSchedule(&next, req); err != nil {
			return nil, err
		}
		tasks.Tasks[i] = next
		if err := s.saveTasks(tasks); err != nil {
			return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
		}
		return &next, nil
	}
	return nil, apperror.New(apperror.CodeNotFound, "task not found")
}

func (s *Scheduler) applyTaskSchedule(task *domainschedule.Task, req schedulerport.AddTaskRequest) error {
	if strings.TrimSpace(req.Task) == "" {
		return apperror.New(apperror.CodeInvalidArgument, "task description is required")
	}
	if len([]byte(strings.TrimSpace(req.Task))) > maxTaskDescriptionBytes {
		return apperror.New(apperror.CodeResourceLimit, "task description is too large")
	}
	if req.CronExpr == "" && req.ExecuteAt == "" {
		return apperror.New(apperror.CodeInvalidArgument, "must provide cron_expr (recurring) or execute_at (one-time)")
	}
	if req.CronExpr != "" && req.ExecuteAt != "" {
		return apperror.New(apperror.CodeInvalidArgument, "cron_expr and execute_at are mutually exclusive")
	}

	task.Task = strings.TrimSpace(req.Task)
	task.CronExpr = ""
	task.OneTime = false
	if req.CronExpr != "" {
		expr := strings.TrimSpace(req.CronExpr)
		nextRun, err := s.ParseCronExpr(expr)
		if err != nil {
			return apperror.Wrap(apperror.CodeInvalidArgument, "invalid cron expression", err)
		}
		task.CronExpr = expr
		task.NextRunAt = nextRun
		return nil
	}

	executeAt, err := time.Parse(time.RFC3339, req.ExecuteAt)
	if err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "invalid time format, use ISO 8601", err)
	}
	if executeAt.Before(time.Now()) {
		return apperror.New(apperror.CodeInvalidArgument, "execute_at must be in the future")
	}
	task.OneTime = true
	task.NextRunAt = executeAt
	return nil
}

// ListTasks 列出调度任务。
func (s *Scheduler) ListTasks(ctx context.Context, statusFilter domainschedule.Status) ([]domainschedule.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	if statusFilter == "" {
		return tasks.Tasks, nil
	}

	var filtered []domainschedule.Task
	for _, task := range tasks.Tasks {
		if task.Status == statusFilter {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

// CancelTask 取消待执行任务，或请求停止正在执行的任务。
func (s *Scheduler) CancelTask(ctx context.Context, taskID string) error {
	if !validTaskID(taskID) {
		return apperror.New(apperror.CodeInvalidArgument, "invalid task ID")
	}

	s.mu.Lock()

	tasks, err := s.loadTasks()
	if err != nil {
		s.mu.Unlock()
		return apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}

	for i := range tasks.Tasks {
		if tasks.Tasks[i].ID != taskID {
			continue
		}
		status := tasks.Tasks[i].Status
		if status == domainschedule.StatusCancelled {
			s.mu.Unlock()
			return nil
		}
		if status != domainschedule.StatusPending && status != domainschedule.StatusRunning {
			s.mu.Unlock()
			return apperror.Errorf(apperror.CodeConflict, "task status is %s and cannot be cancelled", status)
		}
		tasks.Tasks[i].Status = domainschedule.StatusCancelled
		if err := s.saveTasks(tasks); err != nil {
			s.mu.Unlock()
			return apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
		}
		s.mu.Unlock()
		if status == domainschedule.StatusRunning {
			s.CancelExecution(taskID)
		}
		return nil
	}
	s.mu.Unlock()
	return apperror.New(apperror.CodeNotFound, "task not found")
}

// DeleteTask 删除非运行中的任务及其结果。
func (s *Scheduler) DeleteTask(ctx context.Context, taskID string) error {
	if !validTaskID(taskID) {
		return apperror.New(apperror.CodeInvalidArgument, "invalid task ID")
	}
	if err := s.deleteTaskMetadata(taskID); err != nil {
		return err
	}
	if err := os.RemoveAll(s.taskDir(taskID)); err != nil {
		// 任务表是权威状态；目录清理失败留待下次启动回收，避免删除接口变成非幂等。
		log.Printf("[scheduler] cleanup deleted task directory failed: taskID=%s, err=%v", taskID, err)
	}
	return nil
}

func (s *Scheduler) deleteTaskMetadata(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}

	found := false
	remaining := make([]domainschedule.Task, 0, len(tasks.Tasks))
	for _, task := range tasks.Tasks {
		if task.ID != taskID {
			remaining = append(remaining, task)
			continue
		}
		if task.Status == domainschedule.StatusRunning {
			return apperror.New(apperror.CodeConflict, "cannot delete a running task, cancel it first")
		}
		if s.isExecuting(taskID) {
			return apperror.New(apperror.CodeConflict, "cannot delete a task while its execution is stopping")
		}
		found = true
	}
	if !found {
		return apperror.New(apperror.CodeNotFound, "task not found")
	}

	tasks.Tasks = remaining
	if err := s.saveTasks(tasks); err != nil {
		return apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	return nil
}

// SetExecutor 设置任务执行器
func (s *Scheduler) SetExecutor(executor schedulerport.TaskExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
}

// Start 启动调度器后台轮询
func (s *Scheduler) Start() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	s.mu.Lock()
	if s.running || s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopCh = make(chan struct{})
	s.runCtx, s.runCancel = context.WithCancel(context.Background())
	s.running = true
	s.mu.Unlock()

	s.recoverStaleRunningTasks()

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.run()
	}()
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
		if tasks.Tasks[i].Status == domainschedule.StatusRunning {
			tasks.Tasks[i].Status = domainschedule.StatusPending
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
func (s *Scheduler) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.lifecycleMu.Lock()
	s.mu.Lock()
	var runCancel context.CancelFunc
	if s.running {
		close(s.stopCh)
		s.running = false
		runCancel = s.runCancel
		s.runCancel = nil
		s.stopping = true
		s.stopDone = make(chan struct{})
	}
	done := s.stopDone
	s.mu.Unlock()
	s.lifecycleMu.Unlock()
	if done == nil {
		return nil
	}
	if runCancel != nil {
		runCancel()
	}

	// 取消所有正在执行的任务
	s.cancelsMu.Lock()
	for taskID, cancel := range s.cancelFuncs {
		log.Printf("[scheduler] cancelling running task: %s", taskID)
		cancel()
	}
	s.cancelsMu.Unlock()
	if runCancel != nil {
		go func(done chan struct{}) {
			s.wg.Wait()
			s.mu.Lock()
			s.runCtx = nil
			s.stopping = false
			close(done)
			s.mu.Unlock()
		}(done)
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for scheduler shutdown: %w", ctx.Err())
	}
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
		if t.Status != domainschedule.StatusPending {
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
		if task.Status != domainschedule.StatusPending {
			continue
		}
		if now.Before(task.NextRunAt) {
			continue
		}

		// 先占用并发名额，满载时保持 pending，等待下一轮检查。
		select {
		case s.semaphore <- struct{}{}:
		default:
			log.Printf("[scheduler] max concurrent reached (%d), task %s deferred", maxConcurrentTasks, task.ID)
			continue
		}

		// 加写锁二次确认状态，防止重复执行
		s.mu.Lock()
		if !s.running {
			s.mu.Unlock()
			<-s.semaphore
			return
		}
		currentTasks, loadErr := s.loadTasks()
		if loadErr != nil {
			s.mu.Unlock()
			<-s.semaphore
			log.Printf("[scheduler] re-check load failed: %v", loadErr)
			continue
		}

		var currentTask *domainschedule.Task
		for j := range currentTasks.Tasks {
			if currentTasks.Tasks[j].ID == task.ID {
				currentTask = &currentTasks.Tasks[j]
				break
			}
		}

		if currentTask == nil || currentTask.Status != domainschedule.StatusPending || time.Now().Before(currentTask.NextRunAt) {
			s.mu.Unlock()
			<-s.semaphore
			continue
		}

		currentTask.Status = domainschedule.StatusRunning
		currentTask.LastRunAt = &now
		if saveErr := s.saveTasks(currentTasks); saveErr != nil {
			s.mu.Unlock()
			<-s.semaphore
			log.Printf("[scheduler] save task status failed: %v", saveErr)
			continue
		}
		parentCtx := s.runCtx
		if parentCtx == nil {
			parentCtx = context.Background()
		}
		executionCtx, executionCancel := context.WithTimeout(parentCtx, 30*time.Minute)
		s.cancelsMu.Lock()
		s.cancelFuncs[currentTask.ID] = executionCancel
		s.cancelsMu.Unlock()
		s.wg.Add(1)
		s.mu.Unlock()

		go func(ctx context.Context, parent context.Context, cancel context.CancelFunc, tID, tContent, tCron string, tOneTime bool, tExec schedulerport.TaskExecutor) {
			defer s.wg.Done()
			defer func() { <-s.semaphore }()
			defer cancel()
			defer s.finishExecution(tID)
			s.executeTask(ctx, parent, tID, tContent, tCron, tOneTime, tExec)
		}(executionCtx, parentCtx, executionCancel, currentTask.ID, currentTask.Task, currentTask.CronExpr, currentTask.OneTime, executor)
	}
}

func (s *Scheduler) executeTask(ctx context.Context, parentCtx context.Context, taskID string, taskContent string, cronExpr string, oneTime bool, executor schedulerport.TaskExecutor) {
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

			if tasks.Tasks[i].Status == domainschedule.StatusCancelled {
				log.Printf("[scheduler] task cancelled: %s", taskID)
			} else if tasks.Tasks[i].Status != domainschedule.StatusRunning {
				log.Printf("[scheduler] task status changed during execution: taskID=%s, status=%s", taskID, tasks.Tasks[i].Status)
			} else if parentCtx.Err() != nil {
				tasks.Tasks[i].Status = domainschedule.StatusPending
				log.Printf("[scheduler] task interrupted by shutdown, returning to pending: %s", taskID)
			} else if err != nil {
				tasks.Tasks[i].Status = domainschedule.StatusFailed
				log.Printf("[scheduler] task failed: %s, err=%v", taskID, err)
			} else {
				if oneTime {
					tasks.Tasks[i].Status = domainschedule.StatusCompleted
				} else {
					nextRun, cronErr := s.ComputeNextRun(cronExpr, now)
					if cronErr != nil {
						tasks.Tasks[i].Status = domainschedule.StatusFailed
						log.Printf("[scheduler] cron parse failed: taskID=%s, err=%v", taskID, cronErr)
					} else {
						// 避免紧贴当前时间重复触发
						if nextRun.Sub(now) < 30*time.Second {
							nextRun, cronErr = s.ComputeNextRun(cronExpr, nextRun)
							if cronErr != nil {
								tasks.Tasks[i].Status = domainschedule.StatusFailed
								log.Printf("[scheduler] cron parse failed (skip): taskID=%s, err=%v", taskID, cronErr)
								break
							}
						}
						tasks.Tasks[i].Status = domainschedule.StatusPending
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

	tasks, err := s.loadTasks()
	if err != nil {
		s.mu.Unlock()
		return
	}

	cutoff := time.Now().Add(-taskResultTTL)
	remaining := make([]domainschedule.Task, 0, len(tasks.Tasks))
	removedIDs := make([]string, 0)

	for _, t := range tasks.Tasks {
		if t.Status == domainschedule.StatusCompleted || t.Status == domainschedule.StatusFailed || t.Status == domainschedule.StatusCancelled {
			refTime := t.LastRunAt
			if refTime == nil {
				refTime = &t.CreatedAt
			}
			if refTime.Before(cutoff) {
				removedIDs = append(removedIDs, t.ID)
				continue
			}
		}
		remaining = append(remaining, t)
	}

	if len(removedIDs) > 0 {
		tasks.Tasks = remaining
		if err := s.saveTasks(tasks); err != nil {
			s.mu.Unlock()
			log.Printf("[scheduler] save after cleanup failed: %v", err)
			return
		}
	}
	s.mu.Unlock()

	for _, taskID := range removedIDs {
		if err := os.RemoveAll(s.taskDir(taskID)); err != nil {
			log.Printf("[scheduler] cleanup task dir failed: taskID=%s, err=%v", taskID, err)
		}
	}
	if len(removedIDs) > 0 {
		log.Printf("[scheduler] cleaned up %d expired tasks", len(removedIDs))
	}
}

func (s *Scheduler) cleanupOrphanTaskDirs(tasks *domainschedule.TaskList) {
	active := make(map[string]struct{}, len(tasks.Tasks))
	for _, task := range tasks.Tasks {
		active[task.ID] = struct{}{}
	}
	entries, err := os.ReadDir(s.resultsDir)
	if err != nil {
		log.Printf("[scheduler] list result directories for cleanup failed: %v", err)
		return
	}
	if len(entries) > maxResultDirectories {
		log.Printf("[scheduler] skip orphan cleanup: too many result entries (%d)", len(entries))
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validTaskID(entry.Name()) {
			continue
		}
		if _, exists := active[entry.Name()]; exists {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.resultsDir, entry.Name())); err != nil {
			log.Printf("[scheduler] cleanup orphan task directory failed: taskID=%s, err=%v", entry.Name(), err)
		}
	}
}

// CancelExecution 取消正在执行的任务
func (s *Scheduler) CancelExecution(taskID string) {
	s.cancelsMu.Lock()
	if cancel, ok := s.cancelFuncs[taskID]; ok {
		cancel()
		log.Printf("[scheduler] task execution cancelled: %s", taskID)
	}
	s.cancelsMu.Unlock()
}

func (s *Scheduler) isExecuting(taskID string) bool {
	s.cancelsMu.Lock()
	defer s.cancelsMu.Unlock()
	_, ok := s.cancelFuncs[taskID]
	return ok
}

func (s *Scheduler) finishExecution(taskID string) {
	s.cancelsMu.Lock()
	delete(s.cancelFuncs, taskID)
	s.cancelsMu.Unlock()
}

// loadTaskByID 在持有锁的情况下根据 ID 查找任务
func (s *Scheduler) loadTaskByID(taskID string) (*domainschedule.Task, error) {
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

func (s *Scheduler) loadTasks() (*domainschedule.TaskList, error) {
	file, err := os.Open(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task list: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		file.Close()
		return nil, fmt.Errorf("failed to inspect task list: %w", statErr)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("task list is not a regular file")
	}
	if info.Size() > maxTaskStoreBytes {
		file.Close()
		return nil, fmt.Errorf("task list exceeds size limit")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxTaskStoreBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("failed to read task list: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("failed to close task list: %w", closeErr)
	}
	if int64(len(data)) > maxTaskStoreBytes {
		return nil, fmt.Errorf("task list exceeds size limit")
	}

	var list domainschedule.TaskList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to parse task list: %w", err)
	}

	if list.Tasks == nil {
		list.Tasks = []domainschedule.Task{}
	}
	if err := s.validateTaskList(&list); err != nil {
		return nil, err
	}
	return &list, nil
}

func (s *Scheduler) saveTasks(list *domainschedule.TaskList) error {
	if list == nil {
		return fmt.Errorf("task list is nil")
	}
	if list.Tasks == nil {
		list.Tasks = []domainschedule.Task{}
	}
	if err := s.validateTaskList(list); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task list: %w", err)
	}
	if int64(len(data)) > maxTaskStoreBytes {
		return fmt.Errorf("task list exceeds size limit")
	}

	if err := atomicfile.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write task list: %w", err)
	}

	return nil
}

func (s *Scheduler) validateTaskList(list *domainschedule.TaskList) error {
	if len(list.Tasks) > maxScheduledTasks {
		return fmt.Errorf("task count exceeds limit")
	}
	seen := make(map[string]struct{}, len(list.Tasks))
	for _, task := range list.Tasks {
		if !validTaskID(task.ID) {
			return fmt.Errorf("task has invalid ID")
		}
		if _, exists := seen[task.ID]; exists {
			return fmt.Errorf("task list contains duplicate ID %q", task.ID)
		}
		seen[task.ID] = struct{}{}
		if !domainschedule.ValidStatus(task.Status) {
			return fmt.Errorf("task %q has invalid status", task.ID)
		}
		if strings.TrimSpace(task.Task) == "" {
			return fmt.Errorf("task %q has an empty description", task.ID)
		}
		if len([]byte(task.Task)) > maxTaskDescriptionBytes {
			return fmt.Errorf("task %q description exceeds size limit", task.ID)
		}
	}
	return nil
}

// ReadTaskResult 读取任务执行结果。
func (s *Scheduler) ReadTaskResult(ctx context.Context, taskID string) (string, error) {
	if !validTaskID(taskID) {
		return "", apperror.New(apperror.CodeInvalidArgument, "invalid task ID")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, err := s.loadTaskByID(taskID)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	if task == nil {
		return "", apperror.Errorf(apperror.CodeNotFound, "task not found: %s", taskID)
	}
	resultPath := s.taskResultPath(taskID)
	if _, err := os.Stat(resultPath); os.IsNotExist(err) {
		return "", apperror.Errorf(apperror.CodeConflict, "task %s has no result yet", taskID)
	}

	data, err := os.ReadFile(resultPath)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeUnavailable, "scheduler result unavailable", err)
	}
	return string(data), nil
}

// ListHistoryEntries 列出任务的历史结果文件。
func (s *Scheduler) ListHistoryEntries(ctx context.Context, taskID string) ([]domainschedule.HistoryEntry, error) {
	if !validTaskID(taskID) {
		return nil, apperror.New(apperror.CodeInvalidArgument, "invalid task ID")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, err := s.loadTaskByID(taskID)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	if task == nil {
		return nil, apperror.New(apperror.CodeNotFound, "task not found")
	}

	historyDir := filepath.Join(s.taskDir(taskID), "history")
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, apperror.Wrap(apperror.CodeUnavailable, "scheduler history unavailable", err)
	}

	var result []domainschedule.HistoryEntry
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		name := entry.Name()
		timeStr := ""
		if len(name) >= 15 {
			timeStr = fmt.Sprintf("%s-%s-%s %s:%s:%s",
				name[0:4], name[4:6], name[6:8],
				name[9:11], name[11:13], name[13:15])
		}
		result = append(result, domainschedule.HistoryEntry{
			Filename: name,
			Time:     timeStr,
		})
	}
	return result, nil
}

// ReadHistoryFile 读取指定历史结果文件。
func (s *Scheduler) ReadHistoryFile(ctx context.Context, taskID string, filename string) (string, error) {
	if !validTaskID(taskID) {
		return "", apperror.New(apperror.CodeInvalidArgument, "invalid task ID")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, err := s.loadTaskByID(taskID)
	if err != nil {
		return "", apperror.Wrap(apperror.CodeUnavailable, "scheduler storage unavailable", err)
	}
	if task == nil {
		return "", apperror.New(apperror.CodeNotFound, "task not found")
	}

	if filepath.Base(filename) != filename || strings.Contains(filename, `\`) || filepath.Ext(filename) != ".md" {
		return "", apperror.New(apperror.CodeInvalidArgument, "invalid file type")
	}

	filePath := filepath.Join(s.taskDir(taskID), "history", filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", apperror.New(apperror.CodeNotFound, "history file not found")
		}
		return "", apperror.Wrap(apperror.CodeUnavailable, "scheduler history unavailable", err)
	}
	return string(data), nil
}

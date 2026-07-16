package filecron

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainschedule "fkteams/internal/domain/schedule"
	schedulerport "fkteams/internal/ports/scheduler"
)

func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	s, err := NewScheduler(t.TempDir())
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}
	return s
}

func TestAddListCancelDeleteTask(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()

	task, err := s.AddTask(ctx, schedulerport.AddTaskRequest{
		Task:      "生成日报",
		ExecuteAt: time.Now().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if task.ID == "" || task.Status != domainschedule.StatusPending || !task.OneTime {
		t.Fatalf("task = %#v", task)
	}

	pending, err := s.ListTasks(ctx, domainschedule.StatusPending)
	if err != nil {
		t.Fatalf("ListTasks pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != task.ID {
		t.Fatalf("pending tasks = %#v, want %s", pending, task.ID)
	}

	if err := s.CancelTask(ctx, task.ID); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if err := s.CancelTask(ctx, task.ID); err != nil {
		t.Fatalf("repeated CancelTask should be idempotent: %v", err)
	}
	cancelled, err := s.ListTasks(ctx, domainschedule.StatusCancelled)
	if err != nil {
		t.Fatalf("ListTasks cancelled: %v", err)
	}
	if len(cancelled) != 1 || cancelled[0].ID != task.ID {
		t.Fatalf("cancelled tasks = %#v, want %s", cancelled, task.ID)
	}

	if err := os.MkdirAll(s.taskDir(task.ID), 0755); err != nil {
		t.Fatalf("create task dir: %v", err)
	}
	if err := s.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := os.Stat(s.taskDir(task.ID)); !os.IsNotExist(err) {
		t.Fatalf("task dir still exists or unexpected stat error: %v", err)
	}
}

func TestUpdateTask(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()

	task, err := s.AddTask(ctx, schedulerport.AddTaskRequest{
		Task:      "生成日报",
		ExecuteAt: time.Now().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	updated, err := s.UpdateTask(ctx, task.ID, schedulerport.AddTaskRequest{
		Task:      "生成周报",
		ExecuteAt: time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.ID != task.ID || updated.Task != "生成周报" || updated.Status != domainschedule.StatusPending || !updated.OneTime {
		t.Fatalf("updated task = %#v", updated)
	}

	tasks, err := s.ListTasks(ctx, "")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Task != "生成周报" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestAddTaskValidation(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()

	tests := []struct {
		name string
		req  schedulerport.AddTaskRequest
		want string
	}{
		{name: "missing task", req: schedulerport.AddTaskRequest{ExecuteAt: time.Now().Add(time.Hour).Format(time.RFC3339)}, want: "task description is required"},
		{name: "missing schedule", req: schedulerport.AddTaskRequest{Task: "do work"}, want: "must provide"},
		{name: "mutually exclusive", req: schedulerport.AddTaskRequest{Task: "do work", CronExpr: "* * * * *", ExecuteAt: time.Now().Add(time.Hour).Format(time.RFC3339)}, want: "mutually exclusive"},
		{name: "invalid cron", req: schedulerport.AddTaskRequest{Task: "do work", CronExpr: "bad cron"}, want: "invalid cron expression"},
		{name: "past time", req: schedulerport.AddTaskRequest{Task: "do work", ExecuteAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}, want: "must be in the future"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.AddTask(ctx, tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("AddTask error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestResultAndHistoryReaders(t *testing.T) {
	s := newTestScheduler(t)
	taskID := "task-history"
	resultPath := s.taskResultPath(taskID)
	historyDir := filepath.Join(s.taskDir(taskID), "history")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		t.Fatalf("create history dir: %v", err)
	}
	if err := os.WriteFile(resultPath, []byte("latest result"), 0644); err != nil {
		t.Fatalf("write result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "20260430_150405.md"), []byte("history result"), 0644); err != nil {
		t.Fatalf("write history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "ignore.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}
	if err := s.saveTasks(&domainschedule.TaskList{Tasks: []domainschedule.Task{{
		ID:        taskID,
		Task:      "history task",
		Status:    domainschedule.StatusCompleted,
		CreatedAt: time.Now(),
		OneTime:   true,
	}}}); err != nil {
		t.Fatalf("save tasks: %v", err)
	}

	result, err := s.ReadTaskResult(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ReadTaskResult: %v", err)
	}
	if result != "latest result" {
		t.Fatalf("result = %q, want latest result", result)
	}

	entries, err := s.ListHistoryEntries(context.Background(), taskID)
	if err != nil {
		t.Fatalf("ListHistoryEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Filename != "20260430_150405.md" || entries[0].Time != "2026-04-30 15:04:05" {
		t.Fatalf("entries = %#v", entries)
	}

	if _, err := s.ReadHistoryFile(context.Background(), taskID, "../20260430_150405.md"); err == nil {
		t.Fatal("ReadHistoryFile should reject a non-base filename")
	}
	content, err := s.ReadHistoryFile(context.Background(), taskID, "20260430_150405.md")
	if err != nil {
		t.Fatalf("ReadHistoryFile: %v", err)
	}
	if content != "history result" {
		t.Fatalf("history content = %q, want history result", content)
	}
}

func TestTimingRecoveryExecuteAndCleanup(t *testing.T) {
	s := newTestScheduler(t)
	now := time.Now()

	next, err := s.ComputeNextRun("*/5 * * * *", time.Date(2026, 6, 9, 10, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ComputeNextRun: %v", err)
	}
	if next.Minute()%5 != 0 || !next.After(time.Date(2026, 6, 9, 10, 1, 0, 0, time.UTC)) {
		t.Fatalf("unexpected next run: %s", next)
	}

	if err := s.saveTasks(&domainschedule.TaskList{Tasks: []domainschedule.Task{{
		ID:        "running",
		Task:      "running task",
		Status:    domainschedule.StatusRunning,
		NextRunAt: now,
		CreatedAt: now,
	}}}); err != nil {
		t.Fatalf("save running task: %v", err)
	}
	s.recoverStaleRunningTasks()
	pending, err := s.ListTasks(context.Background(), domainschedule.StatusPending)
	if err != nil {
		t.Fatalf("ListTasks pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "running" {
		t.Fatalf("recovered tasks = %#v", pending)
	}

	old := now.Add(-taskResultTTL - time.Hour)
	if err := s.saveTasks(&domainschedule.TaskList{Tasks: []domainschedule.Task{
		{ID: "ok", Task: "ok task", Status: domainschedule.StatusRunning, OneTime: true, CreatedAt: now},
		{ID: "fail", Task: "fail task", Status: domainschedule.StatusRunning, OneTime: true, CreatedAt: now},
		{ID: "old", Task: "old task", Status: domainschedule.StatusCompleted, OneTime: true, CreatedAt: old, LastRunAt: &old},
	}}); err != nil {
		t.Fatalf("save execute tasks: %v", err)
	}
	if err := os.MkdirAll(s.taskDir("old"), 0755); err != nil {
		t.Fatalf("mkdir old task dir: %v", err)
	}

	s.executeTask(context.Background(), context.Background(), "ok", "ok task", "", true, fakeTaskExecutor{})
	s.executeTask(context.Background(), context.Background(), "fail", "fail task", "", true, fakeTaskExecutor{err: errors.New("boom")})
	tasks, err := s.ListTasks(context.Background(), "")
	if err != nil {
		t.Fatalf("ListTasks all: %v", err)
	}
	statusByID := map[string]domainschedule.Status{}
	for _, task := range tasks {
		statusByID[task.ID] = task.Status
	}
	if statusByID["ok"] != domainschedule.StatusCompleted || statusByID["fail"] != domainschedule.StatusFailed {
		t.Fatalf("statuses = %#v", statusByID)
	}

	s.cleanupExpiredTasks()
	if _, err := os.Stat(s.taskDir("old")); !os.IsNotExist(err) {
		t.Fatalf("old task dir should be cleaned, stat err=%v", err)
	}
}

func TestCancelRunningTaskWaitsForExecutionBeforeDelete(t *testing.T) {
	s := newTestScheduler(t)
	now := time.Now()
	taskID := "running-cancel"
	if err := s.saveTasks(&domainschedule.TaskList{Tasks: []domainschedule.Task{{
		ID:        taskID,
		Task:      "blocking task",
		Status:    domainschedule.StatusPending,
		NextRunAt: now.Add(-time.Second),
		CreatedAt: now,
		OneTime:   true,
	}}}); err != nil {
		t.Fatal(err)
	}

	executor := newBlockingTaskExecutor()
	s.SetExecutor(executor)
	s.Start()
	defer func() {
		executor.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Stop(ctx); err != nil {
			t.Errorf("Stop(): %v", err)
		}
	}()

	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled execution did not start")
	}
	if err := s.CancelTask(context.Background(), taskID); err != nil {
		t.Fatalf("CancelTask(): %v", err)
	}
	select {
	case <-executor.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("running execution did not receive cancellation")
	}
	if err := s.DeleteTask(context.Background(), taskID); err == nil {
		t.Fatal("DeleteTask() should reject a task whose execution is still stopping")
	}

	executor.Release()
	deadline := time.Now().Add(2 * time.Second)
	for s.isExecuting(taskID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.isExecuting(taskID) {
		t.Fatal("execution remained registered after executor returned")
	}
	tasks, err := s.ListTasks(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Status != domainschedule.StatusCancelled {
		t.Fatalf("tasks after cancellation = %#v", tasks)
	}
	if err := s.DeleteTask(context.Background(), taskID); err != nil {
		t.Fatalf("DeleteTask() after execution stopped: %v", err)
	}
}

type fakeTaskExecutor struct {
	err error
}

func (e fakeTaskExecutor) Execute(context.Context, string, string) (string, error) {
	return "ok", e.err
}

type blockingTaskExecutor struct {
	started     chan struct{}
	cancelled   chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	cancelOnce  sync.Once
	releaseOnce sync.Once
}

func newBlockingTaskExecutor() *blockingTaskExecutor {
	return &blockingTaskExecutor{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
}

func (e *blockingTaskExecutor) Execute(ctx context.Context, _ string, _ string) (string, error) {
	e.startOnce.Do(func() { close(e.started) })
	<-ctx.Done()
	e.cancelOnce.Do(func() { close(e.cancelled) })
	<-e.release
	return "", ctx.Err()
}

func (e *blockingTaskExecutor) Release() {
	e.releaseOnce.Do(func() { close(e.release) })
}

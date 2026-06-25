package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackgroundTaskOperations(t *testing.T) {
	resetBackgroundTasksForTest(t)

	cancelled := false
	bgTasksMu.Lock()
	bgTasks["running"] = &backgroundTask{
		command: "sleep 60",
		startAt: time.Now().Add(-time.Second),
		cancel: func() {
			cancelled = true
		},
	}
	bgTasks["done"] = &backgroundTask{
		done:    true,
		command: "echo done",
		startAt: time.Now().Add(-2 * time.Second),
		doneAt:  time.Now(),
		resp: &SmartExecuteResponse{
			Success: true,
			Command: "echo done",
			Stdout:  "done\n",
		},
	}
	bgTasks["stale"] = &backgroundTask{
		done:    true,
		command: "old",
		startAt: time.Now().Add(-2 * bgTaskTTL),
		doneAt:  time.Now().Add(-2 * bgTaskTTL),
		resp:    &SmartExecuteResponse{Success: true},
	}
	bgTasksMu.Unlock()

	tools := NewCommandTools(t.TempDir())
	resp, err := tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskAction: "list"})
	if err != nil {
		t.Fatalf("list background tasks error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("list background tasks success = false: %#v", resp)
	}
	if len(resp.Tasks) != 2 {
		t.Fatalf("expected stale task cleanup and two visible tasks, got %#v", resp.Tasks)
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskID: "running"})
	if err != nil {
		t.Fatalf("query running task error: %v", err)
	}
	if !resp.IsBackground || !resp.Success || resp.TaskID != "running" {
		t.Fatalf("unexpected running task response: %#v", resp)
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskAction: "terminate", TaskID: "running"})
	if err != nil {
		t.Fatalf("terminate task error: %v", err)
	}
	if !resp.Success || !cancelled {
		t.Fatalf("expected terminate success and cancel call, resp=%#v cancelled=%v", resp, cancelled)
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskID: "done"})
	if err != nil {
		t.Fatalf("query done task error: %v", err)
	}
	if !resp.Success || resp.Stdout != "done\n" {
		t.Fatalf("unexpected done task response: %#v", resp)
	}
	bgTasksMu.Lock()
	_, exists := bgTasks["done"]
	bgTasksMu.Unlock()
	if exists {
		t.Fatal("done task should be removed after query")
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskAction: "terminate"})
	if err != nil {
		t.Fatalf("missing task id terminate error: %v", err)
	}
	if resp.ErrorMessage != "task_id is required for terminate action" {
		t.Fatalf("missing task id error = %q", resp.ErrorMessage)
	}
}

func TestBackgroundTaskMissingAndCompletedTerminate(t *testing.T) {
	resetBackgroundTasksForTest(t)

	bgTasksMu.Lock()
	bgTasks["done"] = &backgroundTask{
		done:    true,
		command: "echo done",
		startAt: time.Now(),
		doneAt:  time.Now(),
		resp:    &SmartExecuteResponse{Success: true},
	}
	bgTasksMu.Unlock()

	tools := NewCommandTools(t.TempDir())
	resp, err := tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskID: "missing"})
	if err != nil {
		t.Fatalf("query missing task error: %v", err)
	}
	if resp.ErrorMessage == "" {
		t.Fatalf("expected missing task error, got %#v", resp)
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskAction: "terminate", TaskID: "done"})
	if err != nil {
		t.Fatalf("terminate done task error: %v", err)
	}
	if !resp.Success || resp.WarningMessage == "" {
		t.Fatalf("expected completed terminate warning, got %#v", resp)
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{TaskAction: "unknown"})
	if err != nil {
		t.Fatalf("unknown task action error: %v", err)
	}
	if resp.ErrorMessage != "invalid task operation" {
		t.Fatalf("unknown task action error = %q", resp.ErrorMessage)
	}
}

func TestCleanupTempFiles(t *testing.T) {
	workDir := t.TempDir()
	files := []string{
		"cmd_output_one.txt",
		"bg_stdout_one.txt",
		"bg_stderr_one.txt",
		"keep.txt",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(name), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	CleanupTempFiles(workDir)

	for _, name := range files[:3] {
		if _, err := os.Stat(filepath.Join(workDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, "keep.txt")); err != nil {
		t.Fatalf("keep.txt should remain: %v", err)
	}
}

func resetBackgroundTasksForTest(t *testing.T) {
	t.Helper()

	TerminateAll()
	bgTasksMu.Lock()
	bgTasks = make(map[string]*backgroundTask)
	bgTasksMu.Unlock()
	t.Cleanup(func() {
		TerminateAll()
		bgTasksMu.Lock()
		bgTasks = make(map[string]*backgroundTask)
		bgTasksMu.Unlock()
	})
}

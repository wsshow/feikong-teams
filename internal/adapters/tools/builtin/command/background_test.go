package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fkteams/internal/domain/session"
)

func TestBackgroundLifecycleAndSessionIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	manager := NewManager()
	defer manager.Close()
	dir := t.TempDir()
	tools := NewCommandTools(dir, WithManager(manager))
	ctx := session.WithID(context.Background(), "one")
	resp, err := tools.SmartExecute(ctx, &SmartExecuteRequest{Command: "sleep 10", Background: true, Timeout: 1})
	if err != nil || !resp.Success || resp.TaskID == "" || resp.PID == 0 {
		t.Fatalf("start: %#v %v", resp, err)
	}
	other := session.WithID(context.Background(), "two")
	hidden, err := tools.SmartExecute(other, &SmartExecuteRequest{TaskID: resp.TaskID, TaskAction: "status"})
	if err != nil || hidden.ErrorMessage == "" {
		t.Fatalf("cross-session task access: %#v %v", hidden, err)
	}
	listed, _ := tools.SmartExecute(other, &SmartExecuteRequest{TaskAction: "list"})
	if len(listed.Tasks) != 0 {
		t.Fatalf("cross-session task list: %#v", listed)
	}
	separate := NewCommandTools(t.TempDir(), WithManager(manager))
	hidden, _ = separate.SmartExecute(ctx, &SmartExecuteRequest{TaskID: resp.TaskID})
	if hidden.ErrorMessage == "" {
		t.Fatal("cross-workspace task access")
	}
	manager.mu.Lock()
	task := manager.tasks[resp.TaskID]
	manager.mu.Unlock()
	select {
	case <-task.finished:
	case <-time.After(4 * time.Second):
		t.Fatal("background timeout did not stop process")
	}
	result, _ := tools.SmartExecute(ctx, &SmartExecuteRequest{TaskID: resp.TaskID})
	if result.Success || !strings.Contains(result.ErrorMessage, "timed out") {
		t.Fatalf("timeout: %#v", result)
	}
	// 结果读取可重复，避免重试时丢失任务结果。
	repeated, _ := tools.SmartExecute(ctx, &SmartExecuteRequest{TaskID: resp.TaskID})
	if repeated.ErrorMessage != result.ErrorMessage {
		t.Fatal("result was consumed by first read")
	}
}

func TestManagerCloseWaitsAndOnlyRemovesOwnedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell fixture")
	}
	dir := t.TempDir()
	keep := filepath.Join(dir, "cmd_output_user.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	tools := NewCommandTools(dir, WithManager(manager))
	resp, err := tools.SmartExecute(context.Background(), &SmartExecuteRequest{Command: "sleep 20", Background: true})
	if err != nil || !resp.Success {
		t.Fatalf("start: %#v %v", resp, err)
	}
	manager.mu.Lock()
	task := manager.tasks[resp.TaskID]
	manager.mu.Unlock()
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-task.finished:
	default:
		t.Fatal("Close returned before process exit")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unowned file was removed: %v", err)
	}
	resp, _ = tools.SmartExecute(context.Background(), &SmartExecuteRequest{Command: "echo closed"})
	if resp.Success {
		t.Fatal("closed manager accepted new command")
	}
}

func TestBackgroundRejectsCancelledAndOverCapacityBeforeStarting(t *testing.T) {
	tools := NewCommandTools(t.TempDir())
	defer tools.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := tools.SmartExecute(ctx, &SmartExecuteRequest{Command: "echo cancelled", Background: true})
	if err != nil || resp.Success {
		t.Fatalf("cancelled start: %#v %v", resp, err)
	}
	tools.manager.mu.Lock()
	for i := range maxBackgroundTasks {
		tools.manager.tasks[fmt.Sprint(i)] = &backgroundTask{}
	}
	tools.manager.mu.Unlock()
	marker := filepath.Join(tools.workDir, "started")
	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{Command: "touch started", Background: true})
	if err != nil || resp.Success {
		t.Fatalf("capacity: %#v %v", resp, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("capacity rejection happened after process start")
	}
}

func TestTaskOperationValidation(t *testing.T) {
	tools := NewCommandTools(t.TempDir())
	defer tools.Close()
	for _, req := range []*SmartExecuteRequest{{TaskID: "missing"}, {TaskAction: "terminate"}, {TaskAction: "invalid"}} {
		resp, err := tools.SmartExecute(context.Background(), req)
		if err != nil || resp.ErrorMessage == "" {
			t.Fatalf("operation: %#v %v", resp, err)
		}
	}
}

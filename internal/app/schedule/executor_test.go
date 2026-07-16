package schedule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	domainschedule "fkteams/internal/domain/schedule"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestBackgroundExecutorExecuteWritesResult(t *testing.T) {
	resultsDir := t.TempDir()
	executor, err := NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) {
		return fakeRunner{content: "执行完成"}, nil
	}, resultsDir)
	if err != nil {
		t.Fatal(err)
	}

	output, err := executor.Execute(context.Background(), "task-1", "生成报告")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(output, "执行完成") {
		t.Fatalf("output = %q", output)
	}
	resultPath := executor.taskResultPath("task-1")
	content, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read result file: %v", err)
	}
	for _, want := range []string{"# Task Result", "task-1", "生成报告", "执行完成"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("result file missing %q: %s", want, string(content))
		}
	}
	entries, err := os.ReadDir(filepath.Join(executor.taskDir("task-1"), "history"))
	if err != nil {
		t.Fatalf("read history dir: %v", err)
	}
	if len(entries) != 1 || filepath.Ext(entries[0].Name()) != ".md" {
		t.Fatalf("history entries = %#v", entries)
	}
}

func TestBackgroundExecutorExecuteErrorPaths(t *testing.T) {
	executor, err := NewBackgroundExecutor(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), "task-nil", "task"); err == nil {
		t.Fatal("expected nil runner creator error")
	}

	createErr := errors.New("create failed")
	executor, err = NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) {
		return nil, createErr
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), "task-create", "task"); !errors.Is(err, createErr) {
		t.Fatalf("create runner error = %v", err)
	}

	runErr := errors.New("run failed")
	executor, err = NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) {
		return fakeRunner{err: runErr}, nil
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), "task-run", "task"); !errors.Is(err, runErr) {
		t.Fatalf("run error = %v", err)
	}
	content, err := os.ReadFile(executor.taskResultPath("task-run"))
	if err != nil {
		t.Fatalf("read error result: %v", err)
	}
	if !strings.Contains(string(content), "execution error") {
		t.Fatalf("error result content = %s", string(content))
	}
}

func TestNewBackgroundExecutorReportsInitializationFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results")
	if err := os.WriteFile(path, []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewBackgroundExecutor(nil, path); err == nil {
		t.Fatal("NewBackgroundExecutor() should reject a file result path")
	}
}

func TestBackgroundExecutorPrunesHistory(t *testing.T) {
	executor, err := NewBackgroundExecutor(nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	historyDir := filepath.Join(executor.taskDir("task-prune"), "history")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= domainschedule.MaxHistoryEntries; i++ {
		name := fmt.Sprintf("20260101_%06d.md", i)
		if err := os.WriteFile(filepath.Join(historyDir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := executor.writeResult("task-prune", "task", "result"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(historyDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > domainschedule.MaxHistoryEntries {
		t.Fatalf("history entries = %d, want <= %d", len(entries), domainschedule.MaxHistoryEntries)
	}
}

func TestBackgroundExecutorRejectsSymlinkResultDirectory(t *testing.T) {
	resultsDir := t.TempDir()
	executor, err := NewBackgroundExecutor(nil, resultsDir)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, executor.taskDir("task-linked")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := executor.writeResult("task-linked", "task", "secret"); err == nil {
		t.Fatal("writeResult() should reject a symlink task directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("result escaped scheduler root: %#v", entries)
	}
}

type fakeRunner struct {
	content string
	err     error
}

func (r fakeRunner) Run(_ context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	if opts.Sink != nil {
		_ = opts.Sink(event.Event{Type: event.TypeAssistantStarted})
		_ = opts.Sink(event.Event{Type: event.TypeAssistantText, Content: r.content, DeltaKind: event.DeltaOutput})
		_ = opts.Sink(event.Event{Type: event.TypeAssistantCompleted, Content: r.content})
	}
	return &runtimeport.RunResult{LastEvent: event.Event{Type: event.TypeAssistantCompleted}}, nil
}

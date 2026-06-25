package schedule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestBackgroundExecutorExecuteWritesResult(t *testing.T) {
	resultsDir := t.TempDir()
	executor := NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) {
		return fakeRunner{content: "执行完成"}, nil
	}, resultsDir)

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
	executor := NewBackgroundExecutor(nil, t.TempDir())
	if _, err := executor.Execute(context.Background(), "task-nil", "task"); err == nil {
		t.Fatal("expected nil runner creator error")
	}

	createErr := errors.New("create failed")
	executor = NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) {
		return nil, createErr
	}, t.TempDir())
	if _, err := executor.Execute(context.Background(), "task-create", "task"); !errors.Is(err, createErr) {
		t.Fatalf("create runner error = %v", err)
	}

	runErr := errors.New("run failed")
	executor = NewBackgroundExecutor(func(context.Context) (runtimeport.Runner, error) {
		return fakeRunner{err: runErr}, nil
	}, t.TempDir())
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

type fakeRunner struct {
	content string
	err     error
}

func (r fakeRunner) Run(_ context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if r.err != nil {
		return nil, r.err
	}
	if opts.Sink != nil {
		_ = opts.Sink(event.Event{Type: event.TypeMessageStart})
		_ = opts.Sink(event.Event{Type: event.TypeMessageDelta, Content: r.content, DeltaKind: event.DeltaOutput})
		_ = opts.Sink(event.Event{Type: event.TypeMessageEnd, Content: r.content})
	}
	return &runtimeport.RunResult{LastEvent: event.Event{Type: event.TypeMessageEnd}}, nil
}

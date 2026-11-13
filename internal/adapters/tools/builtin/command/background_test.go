package command

import (
	"context"
	localprocess "fkteams/internal/adapters/process/local"
	appprocess "fkteams/internal/app/process"
	domain "fkteams/internal/domain/process"
	"fkteams/internal/domain/session"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTestTools(t *testing.T, opts ...Option) *CommandTools {
	t.Helper()
	backend, err := localprocess.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := appprocess.NewService(backend, backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Error(err)
		}
		if err := backend.Close(); err != nil {
			t.Error(err)
		}
	})
	return NewCommandTools(t.TempDir(), append([]Option{WithService(service)}, opts...)...)
}

func TestBackgroundDeadlineAndSessionIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture")
	}
	tools := newTestTools(t)
	ctx := session.WithID(context.Background(), "one")
	resp, err := tools.SmartExecute(ctx, &SmartExecuteRequest{Command: "printf ready; sleep 10", Background: true, Timeout: 1})
	if err != nil || !resp.Success || resp.TaskID == "" || resp.PID == 0 {
		t.Fatalf("start: %#v %v", resp, err)
	}
	other := session.WithID(context.Background(), "two")
	hidden, _ := tools.Read(other, &ReadRequest{TaskID: resp.TaskID})
	if hidden.ErrorMessage == "" {
		t.Fatal("cross-session read allowed")
	}
	listed, _ := tools.List(other, &ListRequest{})
	if len(listed.Tasks) != 0 {
		t.Fatal("cross-session list")
	}
	separate := NewCommandTools(t.TempDir(), WithService(tools.service))
	hidden, _ = separate.Read(ctx, &ReadRequest{TaskID: resp.TaskID})
	if hidden.ErrorMessage == "" {
		t.Fatal("cross-workspace read")
	}
	result, err := tools.Read(ctx, &ReadRequest{TaskID: resp.TaskID, WaitMS: 6000})
	if err != nil || result.Status != domain.TimedOut || result.Success || !strings.Contains(result.Output.Text, "ready") {
		t.Fatalf("deadline: %#v %v", result, err)
	}
	repeated, _ := tools.Read(ctx, &ReadRequest{TaskID: resp.TaskID})
	if repeated.Output.Text != result.Output.Text || repeated.Status != domain.TimedOut {
		t.Fatal("read consumed result")
	}
}

func TestYieldAndLiveCursorReadDoNotChangeServiceLifetime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture")
	}
	tools := newTestTools(t)
	ctx, cancel := context.WithCancel(session.WithID(context.Background(), "one"))
	yield := 20
	// 延迟首批输出，确保调用方取消后仍会继续执行，并覆盖日志尚未就绪的情况。
	resp, err := tools.SmartExecute(ctx, &SmartExecuteRequest{Command: "sleep 0.2; printf hello; sleep 20", Mode: "service", YieldMS: &yield})
	cancel()
	if err != nil || resp.Status != domain.Running || !resp.IsBackground {
		t.Fatalf("yield: %#v %v", resp, err)
	}
	owner := session.WithID(context.Background(), "one")
	cursor := int64(0)
	var result *ReadResponse
	// wait_ms 只等待终态；在有界期限内确认完整输出就绪，再验证分页游标。
	for deadline := time.Now().Add(3 * time.Second); ; {
		result, err = tools.Read(owner, &ReadRequest{TaskID: resp.TaskID, Cursor: &cursor, Limit: 3, WaitMS: 100})
		if err != nil || result.ErrorMessage != "" || result.Status != domain.Running {
			t.Fatalf("live read: response=%+v output=%+v err=%v", result.SmartExecuteResponse, result.Output, err)
		}
		if result.Output.Total >= int64(len("hello")) {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("timed out waiting for live output: %+v", result.Output)
		}
	}
	if result.Output.Text != "hel" || result.Next != 3 {
		t.Fatalf("live read: %+v", result.Output)
	}
	cursor = result.Next
	next, err := tools.Read(owner, &ReadRequest{TaskID: resp.TaskID, Cursor: &cursor})
	if err != nil || next.ErrorMessage != "" || next.Output.Text != "lo" || next.Next != 5 {
		t.Fatalf("cursor: response=%+v output=%+v err=%v", next.SmartExecuteResponse, next.Output, err)
	}
	stopped, _ := tools.Stop(owner, &StopRequest{TaskID: resp.TaskID})
	if !stopped.Success || stopped.Status != domain.Cancelled {
		t.Fatalf("stop: %#v", stopped)
	}
	again, _ := tools.Stop(owner, &StopRequest{TaskID: resp.TaskID})
	if again.Status != domain.Cancelled {
		t.Fatalf("repeated stop: %#v", again)
	}
}

func TestCloseWaitsAndKeepsLogsAndUnownedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture")
	}
	tools := newTestTools(t)
	keep := filepath.Join(tools.workDir, "user.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	resp, _ := tools.SmartExecute(context.Background(), &SmartExecuteRequest{Command: "printf saved; sleep 20", Background: true, Mode: "service"})
	if err := tools.service.Close(); err != nil {
		t.Fatal(err)
	}
	result, _ := tools.Read(context.Background(), &ReadRequest{TaskID: resp.TaskID})
	if result.Status != domain.Interrupted {
		t.Fatalf("shutdown: %#v", result)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal(err)
	}
	after, _ := tools.SmartExecute(context.Background(), &SmartExecuteRequest{Command: "printf forbidden"})
	if after.Success {
		t.Fatal("closed service accepted command")
	}
}

func TestTaskOperationValidation(t *testing.T) {
	tools := newTestTools(t)
	for _, req := range []*SmartExecuteRequest{{TaskID: "missing"}, {TaskAction: "terminate"}, {TaskAction: "invalid"}} {
		resp, err := tools.SmartExecute(context.Background(), req)
		if err != nil || resp.ErrorMessage == "" {
			t.Fatalf("invalid operation: %#v %v", resp, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, _ := tools.SmartExecute(ctx, &SmartExecuteRequest{Command: "printf forbidden", Background: true})
	if resp.Success {
		t.Fatal("cancelled request accepted")
	}
	for _, req := range []*ReadRequest{{TaskID: "missing", Cursor: ptr64(-2)}, {TaskID: "missing", WaitMS: -1}, {TaskID: "missing", WaitMS: 30001}, {TaskID: "missing", Limit: 65537}} {
		resp, _ := tools.Read(context.Background(), req)
		if resp.ErrorMessage == "" {
			t.Fatal("invalid read accepted")
		}
	}
}
func ptr64(n int64) *int64 { return &n }

func TestForegroundCancellationDrainsBeforeReturning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix fixture")
	}
	tools := newTestTools(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	yield := 30000
	resp, err := tools.SmartExecute(ctx, &SmartExecuteRequest{Command: "sleep 20", YieldMS: &yield})
	if err != nil || resp.Status != domain.Cancelled || resp.ExitCode == nil {
		t.Fatalf("cancel: %#v %v", resp, err)
	}
}

func TestNilToolRequestsReturnBusinessErrors(t *testing.T) {
	tools := newTestTools(t)
	execute, _ := tools.SmartExecute(context.Background(), nil)
	read, _ := tools.Read(context.Background(), nil)
	stop, _ := tools.Stop(context.Background(), nil)
	if execute.ErrorMessage == "" || read.ErrorMessage == "" || stop.ErrorMessage == "" {
		t.Fatal("nil request was accepted")
	}
}

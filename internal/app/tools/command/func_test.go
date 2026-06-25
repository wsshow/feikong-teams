package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSmartExecuteValidationAndExitCodeHints(t *testing.T) {
	tools := NewCommandTools(t.TempDir(), WithApprovalMode(ApprovalModeReject))

	resp, err := tools.SmartExecute(context.Background(), &SmartExecuteRequest{})
	if err != nil {
		t.Fatalf("SmartExecute empty command error: %v", err)
	}
	if resp.ErrorMessage != "command is required" {
		t.Fatalf("empty command error = %q", resp.ErrorMessage)
	}

	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{
		Command: "echo ok",
		Reason:  "test timeout validation",
		Timeout: 601,
	})
	if err != nil {
		t.Fatalf("SmartExecute timeout validation error: %v", err)
	}
	if resp.ErrorMessage != "timeout must be <= 600 seconds" {
		t.Fatalf("timeout error = %q", resp.ErrorMessage)
	}

	if runtime.GOOS == "windows" {
		t.Skip("shell pipeline exit semantics are covered on Unix-like platforms")
	}
	resp, err = tools.SmartExecute(context.Background(), &SmartExecuteRequest{
		Command: `grep zzz /dev/null`,
		Reason:  "test grep miss",
		Timeout: 5,
	})
	if err != nil {
		t.Fatalf("SmartExecute grep miss error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected grep miss to report unsuccessful command")
	}
	if resp.ExitCode == nil || *resp.ExitCode != 1 {
		t.Fatalf("grep miss exit code = %v, want 1", resp.ExitCode)
	}
	if !strings.Contains(resp.ErrorMessage, "grep") {
		t.Fatalf("grep miss error message = %q", resp.ErrorMessage)
	}
}

func TestExecutionContextBuildResponse(t *testing.T) {
	workDir := t.TempDir()
	req := &SmartExecuteRequest{Command: "echo hello"}
	ec := newExecutionContext(req, SecurityEvaluation{Level: LevelModerate, Description: "写入文件", Risks: []string{"覆盖文件"}}, time.Second, workDir)
	ec.startTime = time.Now().Add(-10 * time.Millisecond)
	if _, err := ec.stdout.WriteString(strings.Repeat("line\n", 30_000)); err != nil {
		t.Fatalf("write stdout: %v", err)
	}

	resp := ec.buildResponse(nil)
	if !resp.Success {
		t.Fatalf("expected success response, got %#v", resp)
	}
	if resp.ExitCode == nil || *resp.ExitCode != 0 {
		t.Fatalf("exit code = %v, want 0", resp.ExitCode)
	}
	if resp.OutputFilePath == "" || resp.OutputPreview == "" {
		t.Fatalf("expected large output to be saved, got path=%q preview=%q", resp.OutputFilePath, resp.OutputPreview)
	}
	if _, err := os.Stat(filepath.Join(workDir, resp.OutputFilePath)); err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	if !strings.Contains(resp.WarningMessage, "中等风险命令") {
		t.Fatalf("expected moderate warning, got %q", resp.WarningMessage)
	}

	timeoutCtx := newExecutionContext(req, SecurityEvaluation{Level: LevelSafe}, time.Nanosecond, workDir)
	timeoutCtx.startTime = time.Now()
	<-timeoutCtx.cmdCtx.Done()
	timeoutResp := timeoutCtx.buildResponse(context.DeadlineExceeded)
	if timeoutResp.ExitCode == nil || *timeoutResp.ExitCode != -1 {
		t.Fatalf("timeout exit code = %v, want -1", timeoutResp.ExitCode)
	}
	if !strings.Contains(timeoutResp.ErrorMessage, "timed out") {
		t.Fatalf("timeout error message = %q", timeoutResp.ErrorMessage)
	}

	failedCtx := newExecutionContext(req, SecurityEvaluation{Level: LevelSafe}, time.Second, workDir)
	failedCtx.startTime = time.Now()
	failedResp := failedCtx.buildResponse(errors.New("boom"))
	if failedResp.ExitCode == nil || *failedResp.ExitCode != -1 {
		t.Fatalf("failed exit code = %v, want -1", failedResp.ExitCode)
	}
	if !strings.Contains(failedResp.ErrorMessage, "execution failed") {
		t.Fatalf("failed error message = %q", failedResp.ErrorMessage)
	}
}

func TestInterpretExitCode(t *testing.T) {
	tests := []struct {
		name    string
		command string
		code    int
		want    string
	}{
		{name: "diff", command: "diff a b", code: 1, want: "文件存在差异"},
		{name: "curl dns", command: "curl https://example.invalid", code: 6, want: "DNS"},
		{name: "go test", command: "go test ./...", code: 1, want: "测试失败"},
		{name: "not found", command: "missing-command", code: 127, want: "命令未找到"},
		{name: "signal", command: "sleep 1", code: 143, want: "信号 15"},
		{name: "unknown", command: "custom", code: 2, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpretExitCode(tt.command, tt.code)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("interpretExitCode = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("interpretExitCode = %q, want containing %q", got, tt.want)
			}
		})
	}
}

func TestSaveOutputToFileAndShellHelpers(t *testing.T) {
	workDir := t.TempDir()
	content := strings.Repeat("row\n", outputPreviewLines+5)
	relPath, preview, err := saveOutputToFile(content, workDir)
	if err != nil {
		t.Fatalf("saveOutputToFile: %v", err)
	}
	if filepath.IsAbs(relPath) {
		t.Fatalf("output path should be relative, got %q", relPath)
	}
	if !strings.Contains(preview, "完整内容见文件") {
		t.Fatalf("expected truncated preview hint, got %q", preview)
	}
	got, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("output file content mismatch")
	}

	shell, args := buildShellCommand("echo ok")
	if shell == "" || len(args) == 0 {
		t.Fatalf("buildShellCommand returned shell=%q args=%#v", shell, args)
	}
}

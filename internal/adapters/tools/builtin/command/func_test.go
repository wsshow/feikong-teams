package command

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestSmartExecuteValidationAndExitCodeHints(t *testing.T) {
	tools := newTestTools(t, WithApprovalMode(ApprovalModeReject))

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
		Timeout: 86401,
	})
	if err != nil {
		t.Fatalf("SmartExecute timeout validation error: %v", err)
	}
	if resp.ErrorMessage != "timeout must be between 1 and 86400 seconds when specified" {
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

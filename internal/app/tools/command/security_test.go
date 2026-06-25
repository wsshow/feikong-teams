package command

import (
	"context"
	"strings"
	"testing"
)

func TestEvaluateSecurityClassifiesCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want SecurityLevel
	}{
		{name: "safe command", cmd: "echo hello", want: LevelSafe},
		{name: "moderate recursive remove", cmd: "rm -rf ./tmp", want: LevelModerate},
		{name: "dangerous root remove", cmd: "rm -rf /", want: LevelDangerous},
		{name: "dangerous background escape", cmd: "sleep 10 &", want: LevelDangerous},
		{name: "compound command uses highest level", cmd: "echo ok && dd of=/dev/disk0", want: LevelDangerous},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateSecurity(tt.cmd)
			if got.Level != tt.want {
				t.Fatalf("level = %v, want %v (%s)", got.Level, tt.want, got.Description)
			}
		})
	}
}

func TestSplitShellCommandsRespectsQuotesAndOrOperator(t *testing.T) {
	got := splitShellCommands(`echo "a && b"; grep foo file || true | wc -l`)
	want := []string{`echo "a && b"`, "grep foo file || true", "wc -l"}

	if len(got) != len(want) {
		t.Fatalf("segments = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestSmartExecuteRejectsDangerousCommandWithoutRunning(t *testing.T) {
	tools := NewCommandTools(t.TempDir(), WithApprovalMode(ApprovalModeReject))
	resp, err := tools.SmartExecute(context.Background(), &SmartExecuteRequest{
		Command: "rm -rf /",
		Reason:  "test rejection",
	})
	if err != nil {
		t.Fatalf("SmartExecute returned error: %v", err)
	}
	if resp.Success {
		t.Fatalf("Success = true, want false")
	}
	if resp.SecurityLevel != "危险" {
		t.Fatalf("SecurityLevel = %q, want 危险", resp.SecurityLevel)
	}
	if !strings.Contains(resp.ErrorMessage, "命令被拒绝") {
		t.Fatalf("ErrorMessage = %q, want rejection message", resp.ErrorMessage)
	}
}

func TestSmartExecuteSafeCommand(t *testing.T) {
	tools := NewCommandTools(t.TempDir(), WithApprovalMode(ApprovalModeReject))
	resp, err := tools.SmartExecute(context.Background(), &SmartExecuteRequest{
		Command: "printf fkteams",
		Reason:  "test safe execution",
		Timeout: 5,
	})
	if err != nil {
		t.Fatalf("SmartExecute returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("Success = false, error=%q stderr=%q", resp.ErrorMessage, resp.Stderr)
	}
	if resp.Stdout != "fkteams" {
		t.Fatalf("Stdout = %q, want fkteams", resp.Stdout)
	}
	if resp.ExitCode == nil || *resp.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", resp.ExitCode)
	}
}

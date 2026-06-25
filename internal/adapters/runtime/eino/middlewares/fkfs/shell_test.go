package fkfs

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

func TestLocalShellRejectsEmptyAndDangerousCommands(t *testing.T) {
	shell := NewLocalShell(t.TempDir(), time.Second)

	if _, err := shell.Execute(context.Background(), &filesystem.ExecuteRequest{}); err == nil {
		t.Fatal("Execute() error = nil, want empty command error")
	}

	response, err := shell.Execute(context.Background(), &filesystem.ExecuteRequest{Command: "rm -rf /"})
	if err != nil {
		t.Fatalf("Execute dangerous command error = %v", err)
	}
	if response == nil || response.ExitCode == nil || *response.ExitCode != -1 {
		t.Fatalf("dangerous response = %#v, want exit code -1", response)
	}
	if !strings.Contains(response.Output, "命令被拒绝") {
		t.Fatalf("dangerous output = %q, want rejection message", response.Output)
	}
}

func TestLocalShellExecutesCommandAndCombinesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command syntax is platform specific")
	}
	shell := NewLocalShell(t.TempDir(), time.Second)

	response, err := shell.Execute(context.Background(), &filesystem.ExecuteRequest{
		Command: "printf stdout; printf stderr >&2; exit 7",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.ExitCode == nil || *response.ExitCode != 7 {
		t.Fatalf("exit code = %v, want 7", response.ExitCode)
	}
	if response.Output != "stdout\nstderr" {
		t.Fatalf("output = %q, want combined stdout/stderr", response.Output)
	}
}

func TestLocalShellTimesOutCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command syntax is platform specific")
	}
	shell := NewLocalShell(t.TempDir(), 10*time.Millisecond)

	response, err := shell.Execute(context.Background(), &filesystem.ExecuteRequest{Command: "sleep 1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.ExitCode == nil || *response.ExitCode != -1 {
		t.Fatalf("timeout exit code = %v, want -1", response.ExitCode)
	}
}

func TestDangerousCommandDetection(t *testing.T) {
	if !isDangerousCommand("sudo RM -RF /*") {
		t.Fatal("expected dangerous rm command")
	}
	if isDangerousCommand("echo safe") {
		t.Fatal("echo safe should not be dangerous")
	}
}

func TestLimitedWriterTruncatesButReportsConsumedInput(t *testing.T) {
	var buf bytes.Buffer
	writer := &limitedWriter{w: &buf, limit: 5}

	n, err := writer.Write([]byte("abcdef"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 6 {
		t.Fatalf("Write() n = %d, want original length 6", n)
	}
	if buf.String() != "abcde" {
		t.Fatalf("buffer = %q, want truncated content", buf.String())
	}

	n, err = writer.Write([]byte("xyz"))
	if err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if n != 3 {
		t.Fatalf("second Write() n = %d, want original length 3", n)
	}
	if buf.String() != "abcde" {
		t.Fatalf("buffer after second write = %q, want unchanged", buf.String())
	}
}

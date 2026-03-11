package fkfs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
)

// LocalShell 基于本地系统的 filesystem.Shell 实现
type LocalShell struct {
	workDir string
	timeout time.Duration
}

func NewLocalShell(workDir string, timeout time.Duration) *LocalShell {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &LocalShell{
		workDir: workDir,
		timeout: timeout,
	}
}

func (s *LocalShell) Execute(ctx context.Context, input *filesystem.ExecuteRequest) (*filesystem.ExecuteResponse, error) {
	if input.Command == "" {
		return nil, fmt.Errorf("command 不能为空")
	}

	if isDangerousCommand(input.Command) {
		return &filesystem.ExecuteResponse{
			Output:   "命令被拒绝：检测到危险命令",
			ExitCode: intPtr(-1),
		}, nil
	}

	var shell string
	var shellArgs []string
	switch runtime.GOOS {
	case "windows":
		shell = "powershell"
		shellArgs = []string{"-NoProfile", "-NonInteractive", "-Command", input.Command}
	default:
		shell = "/bin/bash"
		shellArgs = []string{"-c", input.Command}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, shell, shellArgs...)
	cmd.Dir = s.workDir

	var stdoutBuf, stderrBuf bytes.Buffer
	const maxOutput = 1024 * 1024 // 1MB
	cmd.Stdout = &limitedWriter{w: &stdoutBuf, limit: maxOutput}
	cmd.Stderr = &limitedWriter{w: &stderrBuf, limit: maxOutput}

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	output := stdoutBuf.String()
	if stderrBuf.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderrBuf.String()
	}

	truncated := stdoutBuf.Len() >= maxOutput || stderrBuf.Len() >= maxOutput

	return &filesystem.ExecuteResponse{
		Output:    output,
		ExitCode:  &exitCode,
		Truncated: truncated,
	}, nil
}

func intPtr(v int) *int { return &v }

func isDangerousCommand(command string) bool {
	cmdLower := strings.ToLower(command)
	dangerous := []string{
		"rm -rf /",
		"rm -rf /*",
		"mkfs",
		"dd if=/dev/zero",
		":(){ :|:& };:",
		"chmod -r 777 /",
		"kill -9 -1",
		"format-volume",
		"clear-disk",
	}
	for _, d := range dangerous {
		if strings.Contains(cmdLower, d) {
			return true
		}
	}
	return false
}

type limitedWriter struct {
	w       *bytes.Buffer
	limit   int
	written int
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	remaining := lw.limit - lw.written
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err = lw.w.Write(p)
	lw.written += n
	return len(p), err
}

var _ filesystem.Shell = (*LocalShell)(nil)

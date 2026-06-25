//go:build !windows

package command

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// 取消时 kill 整个进程组，避免子进程残留
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// startBackgroundProcess 以 nohup 后台方式启动命令，stdout/stderr 写入临时文件。
func startBackgroundProcess(command, workDir string) (*backgroundProcessResult, error) {
	stdoutFile, err := os.CreateTemp(workDir, "bg_stdout_*.txt")
	if err != nil {
		return nil, err
	}
	stdoutPath := stdoutFile.Name()
	stdoutFile.Close()

	stderrFile, err := os.CreateTemp(workDir, "bg_stderr_*.txt")
	if err != nil {
		os.Remove(stdoutPath)
		return nil, err
	}
	stderrPath := stderrFile.Name()
	stderrFile.Close()

	escaped := strings.ReplaceAll(command, "'", `'\''`)
	bgCommand := fmt.Sprintf("nohup bash -c '%s' > %s 2> %s & echo $!",
		escaped, shellQuote(stdoutPath), shellQuote(stderrPath))
	shell, shellArgs := buildShellCommand(bgCommand)

	cmd := exec.Command(shell, shellArgs...)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		os.Remove(stdoutPath)
		os.Remove(stderrPath)
		return nil, err
	}

	pid := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid); err != nil {
		os.Remove(stdoutPath)
		os.Remove(stderrPath)
		return nil, fmt.Errorf("failed to parse PID: %s", strings.TrimSpace(string(output)))
	}
	return &backgroundProcessResult{PID: pid, StdoutFile: stdoutPath, StderrFile: stderrPath}, nil
}

// shellQuote 用单引号包裹路径，转义其中已有的单引号
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

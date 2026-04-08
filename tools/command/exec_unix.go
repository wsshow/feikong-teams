//go:build !windows

package command

import (
	"fmt"
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

// startBackgroundProcess 以 nohup 后台方式启动命令，返回 PID。
func startBackgroundProcess(command, workDir string) (int, error) {
	bgCommand := fmt.Sprintf("nohup %s > /dev/null 2>&1 & echo $!", command)
	shell, shellArgs := buildShellCommand(bgCommand)

	cmd := exec.Command(shell, shellArgs...)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	pid := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse PID from output: %s", strings.TrimSpace(string(output)))
	}
	return pid, nil
}

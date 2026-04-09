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
// 通过将 command 包裹进子 shell（bash -c '...'）后再传给 nohup，可以正确处理
// command 中包含 &&、||、管道、cd 以及末尾 & 等 shell 元字符的情况；若直接拼接
// 则 nohup 只会作用于第一个 token，导致进程在前台运行并阻塞 cmd.Output()。
func startBackgroundProcess(command, workDir string) (int, error) {
	// 用单引号包裹 command，转义其中已有的单引号（' → '\''）
	escaped := strings.ReplaceAll(command, "'", `'\''`)
	bgCommand := fmt.Sprintf("nohup bash -c '%s' > /dev/null 2>&1 & echo $!", escaped)
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

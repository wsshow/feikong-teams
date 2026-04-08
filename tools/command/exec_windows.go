//go:build windows

package command

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	// 取消时 kill 整个进程树，避免子进程残留
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}

// startBackgroundProcess 以 Start-Process 后台方式启动命令，返回 PID。
func startBackgroundProcess(command, workDir string) (int, error) {
	// PowerShell: Start-Process 启动独立进程，-PassThru 获取进程对象以读取 PID
	psCommand := fmt.Sprintf(
		`$p = Start-Process -FilePath "cmd.exe" -ArgumentList "/c %s" -WindowStyle Hidden -PassThru -WorkingDirectory "%s"; $p.Id`,
		strings.ReplaceAll(command, `"`, `\"`),
		workDir,
	)

	cmd := exec.Command("powershell", "-NonInteractive", "-Command", psCommand)
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

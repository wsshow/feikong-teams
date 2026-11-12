//go:build windows

package executil

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// ConfigureProcessGroup 使超时与取消覆盖整个子进程树，并限制输出管道排空等待。
func ConfigureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	cmd.WaitDelay = 2 * time.Second
	if cmd.Cancel == nil {
		return
	}
	cmd.Cancel = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
	}
}

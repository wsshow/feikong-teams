//go:build !windows

package command

import (
	"os/exec"
	"syscall"
)

func setupCmdProcessGroup(cmd *exec.Cmd) {
	// Unix/Linux 下使用 Setpgid 设置进程组 ID
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

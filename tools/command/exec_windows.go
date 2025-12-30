//go:build windows

package command

import (
	"os/exec"
	"syscall"
)

func setupCmdProcessGroup(cmd *exec.Cmd) {
	// Windows 下使用 CreationFlags 创建新进程组
	// CREATE_NEW_PROCESS_GROUP (0x00000200)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

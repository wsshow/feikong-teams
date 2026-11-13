//go:build windows

package local

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.WaitDelay = 2 * time.Second
}
func taskkill(cmd *exec.Cmd, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"/T", "/PID", fmt.Sprint(cmd.Process.Pid)}
	if force {
		args = append(args, "/F")
	}
	return exec.CommandContext(ctx, "taskkill", args...).Run()
}
func terminate(cmd *exec.Cmd) error { return taskkill(cmd, false) }
func kill(cmd *exec.Cmd) error      { return taskkill(cmd, true) }

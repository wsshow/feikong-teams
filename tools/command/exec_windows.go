//go:build windows

package command

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		return exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
	}
}

// startBackgroundProcess 以 Start-Process 后台方式启动命令，stdout/stderr 写入临时文件。
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

	psCommand := fmt.Sprintf(
		`$p = Start-Process -FilePath "cmd.exe" -ArgumentList "/c %s > %s 2> %s" -WindowStyle Hidden -PassThru -WorkingDirectory "%s"; $p.Id`,
		strings.ReplaceAll(command, `"`, `\"`),
		strings.ReplaceAll(stdoutPath, `"`, `\"`),
		strings.ReplaceAll(stderrPath, `"`, `\"`),
		workDir,
	)

	cmd := exec.Command("powershell", "-NonInteractive", "-Command", psCommand)
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

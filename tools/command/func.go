package command

import (
	"bytes"
	"context"
	"errors"
	"fkteams/tools/approval"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ApprovalMode 审批模式
type ApprovalMode int

const (
	ApprovalModeHITL   ApprovalMode = iota // 危险命令触发中断审批
	ApprovalModeReject                     // 危险命令直接拒绝
)

// Option 配置选项
type Option func(*CommandTools)

// WithApprovalMode 设置审批模式
func WithApprovalMode(mode ApprovalMode) Option {
	return func(t *CommandTools) { t.approvalMode = mode }
}

// CommandTools 命令行工具，带安全审批功能
type CommandTools struct {
	workDir      string
	approvalMode ApprovalMode
}

// NewCommandTools 创建命令行工具实例
func NewCommandTools(workDir string, opts ...Option) *CommandTools {
	t := &CommandTools{workDir: workDir}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// SmartExecuteRequest 智能执行请求
type SmartExecuteRequest struct {
	Command    string `json:"command,omitempty" jsonschema:"description=要执行的 shell 命令（执行命令时必填）"`
	Timeout    int    `json:"timeout,omitempty" jsonschema:"description=超时时间（秒）默认60秒最大600秒"`
	Reason     string `json:"reason,omitempty" jsonschema:"description=执行该命令的原因和目的（执行命令时必填）"`
	Background bool   `json:"background,omitempty" jsonschema:"description=是否后台运行。对于需要持续运行的服务类命令（如 HTTP server、watch、tail -f 等）设为 true，命令会立即返回 PID，通过 kill PID 终止"`
	TaskID     string `json:"task_id,omitempty" jsonschema:"description=后台任务ID，配合 task_action 使用"`
	TaskAction string `json:"task_action,omitempty" jsonschema:"description=后台任务操作: list(列出所有任务) status(查询状态) terminate(终止任务)"`
}

// SmartExecuteResponse 智能执行响应
type SmartExecuteResponse struct {
	Success         bool                 `json:"success"`
	Stdout          string               `json:"stdout,omitempty"`
	Stderr          string               `json:"stderr,omitempty"`
	ExitCode        *int                 `json:"exit_code,omitempty" jsonschema:"description=命令退出码，仅命令执行完成时有值"`
	ExecutionTime   string               `json:"execution_time,omitempty"`
	SecurityLevel   string               `json:"security_level,omitempty"`
	Command         string               `json:"command,omitempty"`
	ErrorMessage    string               `json:"error_message,omitempty"`
	WarningMessage  string               `json:"warning_message,omitempty"`
	OutputTruncated bool                 `json:"output_truncated,omitempty" jsonschema:"description=输出内容因超出大小限制被截断，可能需要将输出重定向到文件以获取完整内容"`
	IsBackground    bool                 `json:"is_background,omitempty" jsonschema:"description=命令已转入后台执行，使用返回的 task_id 查询结果"`
	TaskID          string               `json:"task_id,omitempty" jsonschema:"description=后台任务ID"`
	PID             int                  `json:"pid,omitempty" jsonschema:"description=后台进程PID，可通过 kill PID 终止"`
	Tasks           []BackgroundTaskInfo `json:"tasks,omitempty" jsonschema:"description=后台任务列表"`
}

// BackgroundTaskInfo 后台任务信息
type BackgroundTaskInfo struct {
	TaskID      string `json:"task_id"`
	Command     string `json:"command"`
	Status      string `json:"status"`
	ElapsedTime string `json:"elapsed_time"`
}

// intPtr 辅助函数，返回 int 指针
func intPtr(v int) *int { return &v }

// executionContext 单次命令执行的上下文，封装所有执行期间的共享状态
type executionContext struct {
	req       *SmartExecuteRequest
	eval      SecurityEvaluation
	cmdCtx    context.Context
	cancel    context.CancelFunc
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	stdoutLW  *limitedWriter
	stderrLW  *limitedWriter
	startTime time.Time
	timeout   time.Duration
	done      chan error // cmd.Wait() 结果
}

const maxOutputSize = 1 << 20

func newExecutionContext(req *SmartExecuteRequest, eval SecurityEvaluation, timeout time.Duration) *executionContext {
	// 使用独立 context 控制超时，不依赖父 context（后台化时需要继续执行）
	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
	ec := &executionContext{
		req:     req,
		eval:    eval,
		cmdCtx:  cmdCtx,
		cancel:  cancel,
		timeout: timeout,
		done:    make(chan error, 1),
	}
	ec.stdoutLW = &limitedWriter{w: &ec.stdout, limit: maxOutputSize}
	ec.stderrLW = &limitedWriter{w: &ec.stderr, limit: maxOutputSize}
	return ec
}

// buildResponse 构建命令执行响应
func (ec *executionContext) buildResponse(err error) *SmartExecuteResponse {
	elapsed := time.Since(ec.startTime)

	resp := &SmartExecuteResponse{
		Success:         err == nil,
		Command:         ec.req.Command,
		Stdout:          ec.stdout.String(),
		Stderr:          ec.stderr.String(),
		ExecutionTime:   elapsed.Round(time.Millisecond).String(),
		SecurityLevel:   securityLevelName(ec.eval.Level),
		OutputTruncated: ec.stdoutLW.truncated || ec.stderrLW.truncated,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = intPtr(exitErr.ExitCode())
		} else if ec.cmdCtx.Err() != nil {
			resp.ExitCode = intPtr(-1)
			resp.ErrorMessage = fmt.Sprintf("command timed out after %s", ec.timeout)
		} else {
			resp.ExitCode = intPtr(-1)
			resp.ErrorMessage = fmt.Sprintf("execution failed: %v", err)
		}
	} else {
		resp.ExitCode = intPtr(0)
	}

	if ec.eval.Level == LevelModerate {
		resp.WarningMessage = fmt.Sprintf("中等风险命令: %s - %s",
			ec.eval.Description, strings.Join(ec.eval.Risks, "; "))
	}

	if resp.OutputTruncated {
		truncMsg := "输出内容已截断（超出1MB限制），建议将输出重定向到文件获取完整内容"
		if resp.WarningMessage != "" {
			resp.WarningMessage += "; " + truncMsg
		} else {
			resp.WarningMessage = truncMsg
		}
	}

	return resp
}

// SmartExecute 智能执行命令，危险命令根据审批模式处理
func (t *CommandTools) SmartExecute(ctx context.Context, req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
	// 后台任务管理操作
	if req.TaskAction != "" || req.TaskID != "" {
		return t.handleTaskOperation(req)
	}

	if req.Command == "" {
		return &SmartExecuteResponse{ErrorMessage: "command is required"}, nil
	}

	eval := evaluateSecurity(req.Command)

	if eval.Level == LevelDangerous {
		if t.approvalMode == ApprovalModeReject {
			return &SmartExecuteResponse{
				Command:       req.Command,
				SecurityLevel: securityLevelName(eval.Level),
				ErrorMessage: fmt.Sprintf("命令被拒绝：%s — %s",
					eval.Description, strings.Join(eval.Risks, "; ")),
			}, nil
		}

		info := fmt.Sprintf("危险命令需要审批\n  命令: %s\n  原因: %s\n  风险等级: %s\n  风险描述: %s\n  风险详情: %s",
			req.Command, req.Reason,
			securityLevelName(eval.Level), eval.Description,
			strings.Join(eval.Risks, "; "))
		if err := approval.Require(ctx, approval.StoreCommand, req.Command, info); err != nil {
			if errors.Is(err, approval.ErrRejected) {
				return &SmartExecuteResponse{
					Command:       req.Command,
					SecurityLevel: securityLevelName(eval.Level),
					ErrorMessage:  "command rejected by user",
				}, nil
			}
			return nil, err
		}
	}

	if req.Background {
		return t.executeBackground(req, eval)
	}

	return t.executeCommand(ctx, req, eval)
}

func (t *CommandTools) executeCommand(ctx context.Context, req *SmartExecuteRequest, eval SecurityEvaluation) (*SmartExecuteResponse, error) {
	timeout := 60 * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	} else if req.Timeout > 600 {
		return &SmartExecuteResponse{Command: req.Command, ErrorMessage: "timeout must be <= 600 seconds"}, nil
	}

	shell, shellArgs := buildShellCommand(req.Command)
	ec := newExecutionContext(req, eval, timeout)

	cmd := exec.CommandContext(ec.cmdCtx, shell, shellArgs...)
	cmd.Dir = t.workDir
	setupProcessGroup(cmd)
	cmd.Stdout = ec.stdoutLW
	cmd.Stderr = ec.stderrLW

	ec.startTime = time.Now()
	if err := cmd.Start(); err != nil {
		ec.cancel()
		return &SmartExecuteResponse{
			Command:       req.Command,
			SecurityLevel: securityLevelName(eval.Level),
			ExitCode:      intPtr(-1),
			ErrorMessage:  fmt.Sprintf("failed to start command: %v", err),
		}, nil
	}

	go func() { ec.done <- cmd.Wait() }()

	bgTimer := time.NewTimer(backgroundBudget)
	defer bgTimer.Stop()

	select {
	case err := <-ec.done:
		ec.cancel()
		return ec.buildResponse(err), nil
	case <-bgTimer.C:
		// 超过阻塞预算，先检查命令是否恰好完成
		select {
		case err := <-ec.done:
			ec.cancel()
			return ec.buildResponse(err), nil
		default:
		}
		return t.backgroundExecution(ec)
	case <-ctx.Done():
		// 父 context 取消（如用户取消），先检查命令是否恰好完成
		select {
		case err := <-ec.done:
			ec.cancel()
			return ec.buildResponse(err), nil
		default:
		}
		ec.cancel()
		return &SmartExecuteResponse{
			Command:       req.Command,
			SecurityLevel: securityLevelName(eval.Level),
			ExitCode:      intPtr(-1),
			ErrorMessage:  "command cancelled",
		}, nil
	}
}

// backgroundExecution 将当前执行转入后台
func (t *CommandTools) backgroundExecution(ec *executionContext) (*SmartExecuteResponse, error) {
	taskID, _ := ec.registerAndWaitBackground()

	return &SmartExecuteResponse{
		Success:       true,
		Command:       ec.req.Command,
		SecurityLevel: securityLevelName(ec.eval.Level),
		IsBackground:  true,
		TaskID:        taskID,
		WarningMessage: fmt.Sprintf(
			"命令执行超过%s未完成，已转入后台执行。请告知用户任务已在后台运行，稍后可以让你查看任务完成情况。查询时调用 execute 工具传入 {\"task_id\": \"%s\", \"task_action\": \"status\"} 即可，不需要传 command 和 reason。不要主动轮询",
			backgroundBudget, taskID,
		),
	}, nil
}

// executeBackground 立即以后台方式启动命令，返回 PID。
// 平台特定实现在 exec_unix.go / exec_windows.go 中。
func (t *CommandTools) executeBackground(req *SmartExecuteRequest, eval SecurityEvaluation) (*SmartExecuteResponse, error) {
	pid, err := startBackgroundProcess(req.Command, t.workDir)
	if err != nil {
		return &SmartExecuteResponse{
			Command:       req.Command,
			SecurityLevel: securityLevelName(eval.Level),
			ErrorMessage:  fmt.Sprintf("failed to start background command: %v", err),
		}, nil
	}

	return &SmartExecuteResponse{
		Success:       true,
		Command:       req.Command,
		SecurityLevel: securityLevelName(eval.Level),
		IsBackground:  true,
		PID:           pid,
		WarningMessage: fmt.Sprintf(
			"命令已在后台启动，PID: %d。可通过 kill %d 终止，或通过 ps -p %d 检查是否仍在运行。",
			pid, pid, pid,
		),
	}, nil
}

func buildShellCommand(command string) (shell string, args []string) {
	switch runtime.GOOS {
	case "windows":
		utf8Prefix := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $OutputEncoding = [System.Text.Encoding]::UTF8; "
		return "powershell", []string{"-NonInteractive", "-Command", utf8Prefix + command}
	case "darwin", "linux":
		return "/bin/bash", []string{"-l", "-c", command}
	default:
		return "/bin/sh", []string{"-l", "-c", command}
	}
}

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/executil"
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
	manager      *Manager
	workDir      string
	approvalMode ApprovalMode
}

// NewCommandTools 创建命令行工具实例
func NewCommandTools(workDir string, opts ...Option) *CommandTools {
	t := &CommandTools{workDir: workDir}
	for _, opt := range opts {
		opt(t)
	}
	if t.manager == nil {
		t.manager = NewManager()
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
	OutputFilePath  string               `json:"output_file_path,omitempty" jsonschema:"description=完整输出已写入此文件"`
	OutputPreview   string               `json:"output_preview,omitempty" jsonschema:"description=输出内容的预览片段"`
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
	workDir   string
}

const maxOutputSize = 1 << 20

// 输出超过此大小时写入临时文件而非截断
const outputFileThreshold = 100 << 10

// 存文件后返回的预览行数
const outputPreviewLines = 100
const outputPreviewBytes = 8 << 10

func newExecutionContext(parent context.Context, req *SmartExecuteRequest, eval SecurityEvaluation, timeout time.Duration, workDir string) *executionContext {
	// 执行归属任务管理器，前台取消由调用方显式传递，转后台后继续受应用控制。
	cmdCtx, cancel := context.WithTimeout(parent, timeout)
	ec := &executionContext{
		req:     req,
		eval:    eval,
		cmdCtx:  cmdCtx,
		cancel:  cancel,
		timeout: timeout,
		workDir: workDir,
	}
	ec.stdoutLW = &limitedWriter{w: &ec.stdout, limit: maxOutputSize}
	ec.stderrLW = &limitedWriter{w: &ec.stderr, limit: maxOutputSize}
	return ec
}

// buildResponse 构建命令执行响应
func (ec *executionContext) buildResponse(err error) *SmartExecuteResponse {
	elapsed := time.Since(ec.startTime)

	stdout := ec.stdout.String()
	resp := &SmartExecuteResponse{
		Success:         err == nil,
		Command:         ec.req.Command,
		Stderr:          ec.stderr.String(),
		ExecutionTime:   elapsed.Round(time.Millisecond).String(),
		SecurityLevel:   securityLevelName(ec.eval.Level),
		OutputTruncated: ec.stdoutLW.truncated || ec.stderrLW.truncated,
	}

	// 大输出存文件：超过阈值时写入临时文件，返回路径和预览
	artifact := stdout
	stderrOverflow := len(resp.Stderr) > outputPreviewBytes
	if stderrOverflow {
		artifact += "\n[stderr]\n" + resp.Stderr
		resp.Stderr = executil.String([]byte(resp.Stderr[:outputPreviewBytes]), true)
	}
	if len(artifact) > outputFileThreshold || stderrOverflow {
		if filePath, preview, writeErr := saveOutputToFile(artifact, ec.workDir); writeErr == nil {
			resp.OutputFilePath = filePath
			resp.OutputPreview = preview
			// 已落盘，不占用消息上下文
			resp.Stdout = fmt.Sprintf("输出已写入 %s（共 %d 字节）", filePath, len(artifact))
		} else {
			resp.Stdout = executil.String([]byte(stdout[:min(len(stdout), outputPreviewBytes)]), len(stdout) > outputPreviewBytes)
			resp.WarningMessage = "failed to persist command output; preview only"
			resp.OutputTruncated = true
		}
	} else {
		resp.Stdout = stdout
	}

	if err != nil {
		if ec.cmdCtx.Err() != nil {
			resp.ExitCode = intPtr(-1)
			if errors.Is(ec.cmdCtx.Err(), context.DeadlineExceeded) {
				resp.ErrorMessage = fmt.Sprintf("command timed out after %s", ec.timeout)
			} else {
				resp.ErrorMessage = "command cancelled"
			}
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			resp.ExitCode = intPtr(code)
			// 非零退出码语义映射
			if msg := interpretExitCode(ec.req.Command, code); msg != "" {
				resp.ErrorMessage = msg
			}
		} else {
			resp.ExitCode = intPtr(-1)
			resp.ErrorMessage = fmt.Sprintf("execution failed: %v", err)
		}
	} else {
		resp.ExitCode = intPtr(0)
	}

	if ec.eval.Level == LevelModerate {
		if resp.WarningMessage != "" {
			resp.WarningMessage += "; "
		}
		resp.WarningMessage += fmt.Sprintf("中等风险命令: %s - %s",
			ec.eval.Description, strings.Join(ec.eval.Risks, "; "))
	}

	if resp.OutputTruncated {
		truncMsg := "输出内容已截断，建议将输出重定向到文件获取完整内容"
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
	if err := ctx.Err(); err != nil {
		return &SmartExecuteResponse{ErrorMessage: err.Error()}, nil
	}
	// 后台任务管理操作
	if req.TaskAction != "" || req.TaskID != "" {
		return t.handleTaskOperation(ctx, req)
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

		if err := approval.RequireOperation(ctx, approval.Operation{
			StoreName: approval.StoreCommand,
			Key:       req.Command,
			Title:     "Dangerous command requires approval",
			Target:    req.Command,
			Details: []approval.OperationDetail{
				{Name: "Reason", Value: req.Reason},
				{Name: "SecurityLevel", Value: securityLevelName(eval.Level)},
				{Name: "Description", Value: eval.Description},
				{Name: "Risks", Value: strings.Join(eval.Risks, "; ")},
			},
		}); err != nil {
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

	return t.executeCommand(ctx, req, eval)
}

func (t *CommandTools) executeCommand(ctx context.Context, req *SmartExecuteRequest, eval SecurityEvaluation) (*SmartExecuteResponse, error) {
	timeout := 60 * time.Second
	if req.Timeout > 600 || req.Timeout < 0 {
		return &SmartExecuteResponse{ErrorMessage: "timeout must be <= 600 seconds"}, nil
	}
	if req.Timeout > 0 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	task, err := t.manager.start(ctx, t.workDir, req, eval, timeout)
	if err != nil {
		return &SmartExecuteResponse{ErrorMessage: err.Error()}, nil
	}
	if req.Background {
		return task.runningResponse(), nil
	}
	timer := time.NewTimer(backgroundThreshold)
	defer timer.Stop()
	select {
	case <-task.finished:
		return task.result(), nil
	case <-timer.C:
		return task.runningResponse(), nil
	case <-ctx.Done():
		task.cancel()
		<-task.finished
		return task.result(), nil
	}
}

// WithManager 注入同一应用资源生命周期内共享的命令管理器。
func WithManager(manager *Manager) Option {
	return func(t *CommandTools) {
		if manager != nil {
			t.manager = manager
		}
	}
}

// Close 取消并等待所属命令退出，然后清理该管理器创建的输出文件。
func (t *CommandTools) Close() error { return t.manager.Close() }

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

// interpretExitCode 将非零退出码映射为人类可读的解释
func interpretExitCode(command string, code int) string {
	cmd := strings.TrimSpace(strings.ToLower(command))

	matches := func(prefix string) bool { return strings.HasPrefix(cmd, prefix) }

	switch {
	case matches("grep") && code == 1:
		return "grep: 未找到匹配项（此为退出码 1，非错误）"
	case matches("diff") && code == 1:
		return "diff: 文件存在差异（此为退出码 1，非错误）"
	case matches("curl") && code >= 6 && code <= 89:
		return fmt.Sprintf("curl: %s", curlExitMeaning(code))
	case matches("git diff") && code == 1:
		return "git diff: 文件存在差异（此为退出码 1，非错误）"
	case matches("go test") && code == 1:
		return "go test: 测试失败"
	case matches("go build") && code > 0:
		return "go build: 编译失败，请检查错误输出"
	case matches("git grep") && code == 1:
		return "git grep: 未找到匹配项"
	}
	if code >= 126 && code <= 128 {
		return fmt.Sprintf("命令无法执行（退出码 %d：%s）", code, shellExitMeaning(code))
	}
	if code > 128 {
		return fmt.Sprintf("命令被信号 %d 终止", code-128)
	}
	return ""
}

func curlExitMeaning(code int) string {
	switch code {
	case 6:
		return "DNS 解析失败"
	case 7:
		return "连接被拒绝"
	case 28:
		return "连接超时"
	case 35:
		return "TLS 握手失败"
	default:
		return fmt.Sprintf("错误码 %d", code)
	}
}

func shellExitMeaning(code int) string {
	switch code {
	case 126:
		return "命令不可执行（权限问题）"
	case 127:
		return "命令未找到"
	case 128:
		return "exit 参数无效"
	}
	return ""
}

// saveOutputToFile 将大输出写入临时文件，返回文件路径和预览
func saveOutputToFile(content, workDir string) (string, string, error) {
	file, err := os.CreateTemp(workDir, "cmd_output_*.txt")
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		os.Remove(file.Name())
		return "", "", err
	}

	if err := file.Sync(); err != nil {
		os.Remove(file.Name())
		return "", "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		return "", "", err
	}

	// 生成预览：截取前 outputPreviewLines 行
	preview := content
	lines := strings.Split(content, "\n")
	if len(lines) > outputPreviewLines {
		preview = strings.Join(lines[:outputPreviewLines], "\n")
		preview += fmt.Sprintf("\n\n... 已截断（共 %d 行，完整内容见文件）", len(lines))
	}

	if len(preview) > outputPreviewBytes {
		preview = executil.String([]byte(preview[:outputPreviewBytes]), true)
	}

	// 返回相对路径，让 file_read 直接在工作目录内解析，避免外部路径审批
	relPath, _ := filepath.Rel(workDir, file.Name())
	return relPath, preview, nil
}

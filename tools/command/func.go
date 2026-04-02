package command

import (
	"bytes"
	"context"
	"errors"
	"fkteams/tools/approval"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
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
	TaskID     string `json:"task_id,omitempty" jsonschema:"description=后台任务ID，配合 task_action 使用"`
	TaskAction string `json:"task_action,omitempty" jsonschema:"description=后台任务操作: list(列出所有任务) status(查询状态) terminate(终止任务)"`
}

// SmartExecuteResponse 智能执行响应
type SmartExecuteResponse struct {
	Success         bool                 `json:"success"`
	Stdout          string               `json:"stdout,omitempty"`
	Stderr          string               `json:"stderr,omitempty"`
	ExitCode        int                  `json:"exit_code"`
	ExecutionTime   string               `json:"execution_time,omitempty"`
	SecurityLevel   string               `json:"security_level"`
	Command         string               `json:"command"`
	ErrorMessage    string               `json:"error_message,omitempty"`
	WarningMessage  string               `json:"warning_message,omitempty"`
	OutputTruncated bool                 `json:"output_truncated,omitempty" jsonschema:"description=输出内容因超出大小限制被截断，可能需要将输出重定向到文件以获取完整内容"`
	IsBackground    bool                 `json:"is_background,omitempty" jsonschema:"description=命令已转入后台执行，使用返回的 task_id 查询结果"`
	TaskID          string               `json:"task_id,omitempty" jsonschema:"description=后台任务ID"`
	Tasks           []BackgroundTaskInfo `json:"tasks,omitempty" jsonschema:"description=后台任务列表"`
}

// BackgroundTaskInfo 后台任务信息
type BackgroundTaskInfo struct {
	TaskID      string `json:"task_id"`
	Command     string `json:"command"`
	Status      string `json:"status"`
	ElapsedTime string `json:"elapsed_time"`
}

// SecurityLevel 安全等级
type SecurityLevel int

const (
	LevelSafe SecurityLevel = iota
	LevelModerate
	LevelDangerous
)

// SecurityEvaluation 安全评估结果
type SecurityEvaluation struct {
	Level       SecurityLevel
	Description string
	Risks       []string
}

// 危险命令黑名单
var dangerousCommands = map[string]SecurityEvaluation{
	// Unix/Linux
	"rm -rf /":        {Level: LevelDangerous, Description: "删除根目录", Risks: []string{"会导致系统完全崩溃"}},
	"rm -rf /*":       {Level: LevelDangerous, Description: "删除根目录下所有内容", Risks: []string{"会导致系统完全崩溃"}},
	"mkfs":            {Level: LevelDangerous, Description: "格式化文件系统", Risks: []string{"会清除磁盘上的所有数据"}},
	"dd if=/dev/zero": {Level: LevelDangerous, Description: "覆盖设备", Risks: []string{"可能永久性擦除数据"}},
	":(){ :|:& };:":   {Level: LevelDangerous, Description: "fork 炸弹", Risks: []string{"会耗尽系统资源"}},
	"chmod -r 777 /":  {Level: LevelDangerous, Description: "递归修改根目录权限", Risks: []string{"严重的安全风险"}},
	"chown -r":        {Level: LevelDangerous, Description: "递归修改所有者", Risks: []string{"可能破坏系统权限"}},
	"mv /":            {Level: LevelDangerous, Description: "移动根目录", Risks: []string{"会破坏系统结构"}},
	"kill -9 -1":      {Level: LevelDangerous, Description: "杀死所有进程", Risks: []string{"会导致系统崩溃"}},
	// Windows/PowerShell
	"remove-item -recurse -force c:\\": {Level: LevelDangerous, Description: "递归删除系统盘", Risks: []string{"会导致系统完全崩溃"}},
	"format-volume":                    {Level: LevelDangerous, Description: "格式化卷", Risks: []string{"会清除磁盘数据"}},
	"clear-disk":                       {Level: LevelDangerous, Description: "清除磁盘", Risks: []string{"会永久擦除数据"}},
	"stop-process -id 0":               {Level: LevelDangerous, Description: "终止系统关键进程", Risks: []string{"会导致系统崩溃"}},
	"stop-computer":                    {Level: LevelDangerous, Description: "关闭计算机", Risks: []string{"会立即关机"}},
	"restart-computer":                 {Level: LevelDangerous, Description: "重启计算机", Risks: []string{"会立即重启"}},
	"set-executionpolicy unrestricted": {Level: LevelDangerous, Description: "解除脚本执行限制", Risks: []string{"严重安全风险"}},
}

// 需要特别审查的命令模式
var riskyPatterns = []struct {
	Pattern     string
	Level       SecurityLevel
	Description string
	Risk        string
}{
	// Unix/Linux
	{"rm -rf", LevelDangerous, "强制递归删除", "可能意外删除重要文件"},
	{"rm -r", LevelModerate, "递归删除", "可能意外删除文件"},
	{"dd if=", LevelDangerous, "dd 磁盘写入命令", "可能覆盖重要数据"},
	{"> /etc/", LevelDangerous, "重定向到系统配置", "可能破坏系统配置"},
	{"> /", LevelDangerous, "重定向到系统目录", "可能破坏系统文件"},
	{"chmod 777", LevelModerate, "设置全局可写权限", "安全风险"},
	{"chmod -r", LevelModerate, "递归修改权限", "可能影响多个文件"},
	{"chown -r", LevelModerate, "递归修改所有者", "可能破坏权限结构"},
	{"kill -9", LevelModerate, "强制终止进程", "可能导致数据丢失"},
	{"killall", LevelModerate, "终止进程组", "可能导致服务中断"},
	{"pkill", LevelModerate, "终止进程", "可能导致服务中断"},
	{"sudo ", LevelModerate, "以管理员权限执行", "高权限操作"},
	{"pip install", LevelModerate, "安装 Python 包", "可能引入不安全的依赖"},
	{"npm install -g", LevelModerate, "全局安装 npm 包", "可能影响系统环境"},
	{"wget", LevelModerate, "下载文件", "可能下载恶意内容"},
	{"curl", LevelModerate, "下载/上传数据", "可能泄露数据"},
	// Windows/PowerShell
	{"remove-item -recurse -force", LevelDangerous, "强制递归删除", "可能意外删除重要文件"},
	{"remove-item -recurse", LevelModerate, "递归删除", "可能意外删除文件"},
	{"stop-process", LevelModerate, "终止进程", "可能导致服务中断"},
	{"invoke-webrequest", LevelModerate, "下载文件", "可能下载恶意内容"},
	{"invoke-restmethod", LevelModerate, "调用远程接口", "可能泄露数据或下载恶意内容"},
	{"new-psdrive", LevelModerate, "映射网络驱动器", "可能连接不可信网络资源"},
}

func evaluateSecurity(command string) SecurityEvaluation {
	cmdLower := strings.ToLower(strings.TrimSpace(command))

	for pattern, eval := range dangerousCommands {
		if strings.Contains(cmdLower, pattern) {
			return eval
		}
	}

	for _, p := range riskyPatterns {
		if strings.Contains(cmdLower, p.Pattern) {
			return SecurityEvaluation{
				Level:       p.Level,
				Description: p.Description,
				Risks:       []string{p.Risk},
			}
		}
	}

	return SecurityEvaluation{Level: LevelSafe, Description: "常规命令"}
}

func securityLevelName(level SecurityLevel) string {
	switch level {
	case LevelSafe:
		return "安全"
	case LevelModerate:
		return "中等"
	case LevelDangerous:
		return "危险"
	default:
		return "未知"
	}
}

const (
	backgroundBudget = 15 * time.Second
	bgTaskTTL        = 1 * time.Hour // 后台任务结果保留时间
)

// backgroundTask 后台任务
type backgroundTask struct {
	mu      sync.Mutex
	done    bool
	resp    *SmartExecuteResponse
	command string
	startAt time.Time
	doneAt  time.Time
	cancel  context.CancelFunc
}

var (
	bgTasks   = make(map[string]*backgroundTask)
	bgTasksMu sync.Mutex
)

// cleanStaleTasks 清理过期的后台任务，需在持有 bgTasksMu 时调用
func cleanStaleTasks() {
	now := time.Now()
	for id, t := range bgTasks {
		t.mu.Lock()
		if t.done && now.Sub(t.doneAt) > bgTaskTTL {
			t.mu.Unlock()
			delete(bgTasks, id)
			continue
		}
		t.mu.Unlock()
	}
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
		// Reject 模式：直接拒绝
		if t.approvalMode == ApprovalModeReject {
			return &SmartExecuteResponse{
				Command:       req.Command,
				SecurityLevel: securityLevelName(eval.Level),
				ErrorMessage: fmt.Sprintf("命令被拒绝：%s — %s",
					eval.Description, strings.Join(eval.Risks, "; ")),
			}, nil
		}

		// HITL 审批流程
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

	return t.executeCommand(ctx, req, eval)
}

func (t *CommandTools) executeCommand(ctx context.Context, req *SmartExecuteRequest, eval SecurityEvaluation) (*SmartExecuteResponse, error) {
	timeout := 60 * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	} else if req.Timeout > 600 {
		return &SmartExecuteResponse{Command: req.Command, ErrorMessage: "timeout must be <= 600 seconds"}, nil
	}

	var shell string
	var shellArgs []string
	switch runtime.GOOS {
	case "windows":
		shell = "powershell"
		utf8Prefix := "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $OutputEncoding = [System.Text.Encoding]::UTF8; "
		shellArgs = []string{"-NonInteractive", "-Command", utf8Prefix + req.Command}
	case "darwin", "linux":
		shell = "/bin/bash"
		shellArgs = []string{"-l", "-c", req.Command}
	default:
		shell = "/bin/sh"
		shellArgs = []string{"-l", "-c", req.Command}
	}

	// 使用独立 context 控制超时，不依赖父 context（后台化时需要继续执行）
	cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)

	cmd := exec.CommandContext(cmdCtx, shell, shellArgs...)
	cmd.Dir = t.workDir
	setupProcessGroup(cmd)

	const maxOutputSize = 1 << 20
	var stdout, stderr bytes.Buffer
	stdoutLW := &limitedWriter{w: &stdout, limit: maxOutputSize}
	stderrLW := &limitedWriter{w: &stderr, limit: maxOutputSize}
	cmd.Stdout = stdoutLW
	cmd.Stderr = stderrLW

	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		cancel()
		return &SmartExecuteResponse{
			Command:       req.Command,
			SecurityLevel: securityLevelName(eval.Level),
			ExitCode:      -1,
			ErrorMessage:  fmt.Sprintf("failed to start command: %v", err),
		}, nil
	}

	// 等待命令完成，带自动后台化
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	bgTimer := time.NewTimer(backgroundBudget)
	defer bgTimer.Stop()

	select {
	case err := <-done:
		cancel()
		return t.buildResponse(req, eval, err, cmdCtx, &stdout, &stderr, stdoutLW, stderrLW, startTime, timeout)
	case <-bgTimer.C:
		// 超过阻塞时间，先检查命令是否恰好完成
		select {
		case err := <-done:
			cancel()
			return t.buildResponse(req, eval, err, cmdCtx, &stdout, &stderr, stdoutLW, stderrLW, startTime, timeout)
		default:
		}
		return t.backgroundCommand(req, eval, done, cancel, &stdout, &stderr, stdoutLW, stderrLW, startTime, timeout, cmdCtx)
	case <-ctx.Done():
		// 父 context 取消（如用户取消），先检查命令是否恰好完成
		select {
		case err := <-done:
			cancel()
			return t.buildResponse(req, eval, err, cmdCtx, &stdout, &stderr, stdoutLW, stderrLW, startTime, timeout)
		default:
		}
		cancel()
		return &SmartExecuteResponse{
			Command:       req.Command,
			SecurityLevel: securityLevelName(eval.Level),
			ExitCode:      -1,
			ErrorMessage:  "command cancelled",
		}, nil
	}
}

// buildResponse 构建命令执行响应
func (t *CommandTools) buildResponse(
	req *SmartExecuteRequest, eval SecurityEvaluation,
	err error, cmdCtx context.Context,
	stdout, stderr *bytes.Buffer,
	stdoutLW, stderrLW *limitedWriter,
	startTime time.Time, timeout time.Duration,
) (*SmartExecuteResponse, error) {
	elapsed := time.Since(startTime)

	resp := &SmartExecuteResponse{
		Success:         err == nil,
		Command:         req.Command,
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		ExecutionTime:   elapsed.Round(time.Millisecond).String(),
		SecurityLevel:   securityLevelName(eval.Level),
		OutputTruncated: stdoutLW.truncated || stderrLW.truncated,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else if cmdCtx.Err() != nil {
			resp.ExitCode = -1
			resp.ErrorMessage = fmt.Sprintf("command timed out after %s", timeout)
		} else {
			resp.ExitCode = -1
			resp.ErrorMessage = fmt.Sprintf("execution failed: %v", err)
		}
	}

	if eval.Level == LevelModerate {
		resp.WarningMessage = fmt.Sprintf("中等风险命令: %s - %s",
			eval.Description, strings.Join(eval.Risks, "; "))
	}

	if resp.OutputTruncated {
		truncMsg := "输出内容已截断（超出1MB限制），建议将输出重定向到文件获取完整内容"
		if resp.WarningMessage != "" {
			resp.WarningMessage += "; " + truncMsg
		} else {
			resp.WarningMessage = truncMsg
		}
	}

	return resp, nil
}

// backgroundCommand 将命令转入后台执行
func (t *CommandTools) backgroundCommand(
	req *SmartExecuteRequest, eval SecurityEvaluation,
	done <-chan error, cancel context.CancelFunc,
	stdout, stderr *bytes.Buffer,
	stdoutLW, stderrLW *limitedWriter,
	startTime time.Time, timeout time.Duration,
	cmdCtx context.Context,
) (*SmartExecuteResponse, error) {
	taskID := fmt.Sprintf("bg_%d", time.Now().UnixNano())

	task := &backgroundTask{
		command: req.Command,
		startAt: startTime,
		cancel:  cancel,
	}

	bgTasksMu.Lock()
	cleanStaleTasks()
	bgTasks[taskID] = task
	bgTasksMu.Unlock()

	// 后台协程等待命令完成
	go func() {
		defer cancel()
		err := <-done
		resp, _ := t.buildResponse(req, eval, err, cmdCtx, stdout, stderr, stdoutLW, stderrLW, startTime, timeout)
		resp.IsBackground = true
		resp.TaskID = taskID

		task.mu.Lock()
		task.resp = resp
		task.done = true
		task.doneAt = time.Now()
		task.mu.Unlock()
	}()

	return &SmartExecuteResponse{
		Success:       true,
		Command:       req.Command,
		SecurityLevel: securityLevelName(eval.Level),
		IsBackground:  true,
		TaskID:        taskID,
		WarningMessage: fmt.Sprintf(
			"命令执行超过%s未完成，已转入后台执行。请告知用户任务已在后台运行，稍后可以让你查看任务完成情况。查询时调用 execute 工具传入 {\"task_id\": \"%s\", \"task_action\": \"status\"} 即可，不需要传 command 和 reason。不要主动轮询",
			backgroundBudget, taskID,
		),
	}, nil
}

// handleTaskOperation 后台任务管理路由
func (t *CommandTools) handleTaskOperation(req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
	switch req.TaskAction {
	case "list":
		return t.listBackgroundTasks()
	case "terminate":
		if req.TaskID == "" {
			return &SmartExecuteResponse{ErrorMessage: "task_id is required for terminate action"}, nil
		}
		return t.terminateBackgroundTask(req.TaskID)
	default:
		if req.TaskID != "" {
			return t.queryBackgroundTask(req.TaskID)
		}
		return &SmartExecuteResponse{ErrorMessage: "invalid task operation"}, nil
	}
}

// listBackgroundTasks 列出所有后台任务
func (t *CommandTools) listBackgroundTasks() (*SmartExecuteResponse, error) {
	bgTasksMu.Lock()
	cleanStaleTasks()
	tasks := make([]BackgroundTaskInfo, 0, len(bgTasks))
	for id, task := range bgTasks {
		task.mu.Lock()
		info := BackgroundTaskInfo{
			TaskID:      id,
			Command:     task.command,
			ElapsedTime: time.Since(task.startAt).Round(time.Millisecond).String(),
		}
		if task.done {
			if task.resp != nil && task.resp.Success {
				info.Status = "completed"
			} else {
				info.Status = "failed"
			}
		} else {
			info.Status = "running"
		}
		task.mu.Unlock()
		tasks = append(tasks, info)
	}
	bgTasksMu.Unlock()

	resp := &SmartExecuteResponse{Success: true, Tasks: tasks}
	if len(tasks) == 0 {
		resp.WarningMessage = "当前没有后台任务"
	}
	return resp, nil
}

// terminateBackgroundTask 终止后台任务
func (t *CommandTools) terminateBackgroundTask(taskID string) (*SmartExecuteResponse, error) {
	bgTasksMu.Lock()
	task, ok := bgTasks[taskID]
	bgTasksMu.Unlock()

	if !ok {
		return &SmartExecuteResponse{ErrorMessage: fmt.Sprintf("task %q not found", taskID)}, nil
	}

	task.mu.Lock()
	if task.done {
		task.mu.Unlock()
		return &SmartExecuteResponse{
			Success:        true,
			Command:        task.command,
			WarningMessage: "任务已结束，无需终止",
		}, nil
	}
	cancelFn := task.cancel
	task.mu.Unlock()

	// 取消 context 触发 cmd.Cancel 杀死进程组
	if cancelFn != nil {
		cancelFn()
	}

	return &SmartExecuteResponse{
		Success:        true,
		Command:        task.command,
		TaskID:         taskID,
		WarningMessage: "已发送终止信号，任务正在停止",
	}, nil
}

// queryBackgroundTask 查询后台任务结果
func (t *CommandTools) queryBackgroundTask(taskID string) (*SmartExecuteResponse, error) {
	bgTasksMu.Lock()
	task, ok := bgTasks[taskID]
	bgTasksMu.Unlock()

	if !ok {
		return &SmartExecuteResponse{ErrorMessage: fmt.Sprintf("task %q not found", taskID)}, nil
	}

	task.mu.Lock()
	if !task.done {
		elapsed := time.Since(task.startAt).Round(time.Millisecond)
		task.mu.Unlock()
		return &SmartExecuteResponse{
			Success:        true,
			Command:        task.command,
			IsBackground:   true,
			TaskID:         taskID,
			WarningMessage: fmt.Sprintf("任务仍在执行中（已运行 %s），请稍后再查询", elapsed),
		}, nil
	}
	resp := task.resp
	task.mu.Unlock()

	// 任务完成，返回结果并清理
	bgTasksMu.Lock()
	delete(bgTasks, taskID)
	bgTasksMu.Unlock()

	return resp, nil
}

type limitedWriter struct {
	w         io.Writer
	limit     int64
	written   int64
	truncated bool
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	remaining := lw.limit - lw.written
	if remaining <= 0 {
		lw.truncated = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
		lw.truncated = true
	}
	n, err = lw.w.Write(p)
	lw.written += int64(n)
	return len(p), err
}

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
	Command string `json:"command" jsonschema:"description=要执行的 shell 命令,required"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=超时时间（秒）默认60秒最大600秒"`
	Reason  string `json:"reason" jsonschema:"description=执行该命令的原因和目的,required"`
}

// SmartExecuteResponse 智能执行响应
type SmartExecuteResponse struct {
	Success        bool   `json:"success"`
	Stdout         string `json:"stdout,omitempty"`
	Stderr         string `json:"stderr,omitempty"`
	ExitCode       int    `json:"exit_code"`
	ExecutionTime  string `json:"execution_time,omitempty"`
	SecurityLevel  string `json:"security_level"`
	Command        string `json:"command"`
	ErrorMessage   string `json:"error_message,omitempty"`
	WarningMessage string `json:"warning_message,omitempty"`
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

// SmartExecute 智能执行命令，危险命令根据审批模式处理
func (t *CommandTools) SmartExecute(ctx context.Context, req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
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
		shellArgs = []string{"-NoProfile", "-NonInteractive", "-Command", utf8Prefix + req.Command}
	case "darwin", "linux":
		shell = "/bin/bash"
		shellArgs = []string{"-c", req.Command}
	default:
		shell = "/bin/sh"
		shellArgs = []string{"-c", req.Command}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, shell, shellArgs...)
	cmd.Dir = t.workDir
	setupProcessGroup(cmd)

	const maxOutputSize = 1 << 20
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: maxOutputSize}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: maxOutputSize}

	startTime := time.Now()
	err := cmd.Run()
	elapsed := time.Since(startTime)

	resp := &SmartExecuteResponse{
		Success:       err == nil,
		Command:       req.Command,
		Stdout:        stdout.String(),
		Stderr:        stderr.String(),
		ExecutionTime: elapsed.Round(time.Millisecond).String(),
		SecurityLevel: securityLevelName(eval.Level),
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

	return resp, nil
}

type limitedWriter struct {
	w       io.Writer
	limit   int64
	written int64
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	remaining := lw.limit - lw.written
	if remaining <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err = lw.w.Write(p)
	lw.written += int64(n)
	return len(p), err
}

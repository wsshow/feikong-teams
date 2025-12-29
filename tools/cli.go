package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// CommandSecurityLevel 定义命令的安全级别
type CommandSecurityLevel int

const (
	// SecurityLevelSafe 安全命令，只读操作，不会修改系统
	SecurityLevelSafe CommandSecurityLevel = iota
	// SecurityLevelModerate 中等风险命令，可能修改文件或系统设置
	SecurityLevelModerate
	// SecurityLevelDangerous 危险命令，可能造成系统损坏或数据丢失
	SecurityLevelDangerous
)

// CommandDangerousLevel 定义危险等级
type CommandDangerousLevel struct {
	Level       CommandSecurityLevel
	Description string
	Reasons     []string
}

// 危险命令黑名单
var dangerousCommands = map[string]CommandDangerousLevel{
	"rm -rf /":     {Level: SecurityLevelDangerous, Description: "删除根目录", Reasons: []string{"会导致系统完全崩溃"}},
	"rm -rf /*":    {Level: SecurityLevelDangerous, Description: "删除根目录下所有内容", Reasons: []string{"会导致系统完全崩溃"}},
	"mkfs":         {Level: SecurityLevelDangerous, Description: "格式化文件系统", Reasons: []string{"会清除磁盘上的所有数据"}},
	"dd if=/dev/zero": {Level: SecurityLevelDangerous, Description: "使用 dd 命令覆盖设备", Reasons: []string{"可能永久性擦除数据"}},
	":(){ :|:& };:": {Level: SecurityLevelDangerous, Description: "fork 炸弹", Reasons: []string{"会耗尽系统资源"}},
	"chmod -R 777 /": {Level: SecurityLevelDangerous, Description: "递归修改根目录权限", Reasons: []string{"严重的安全风险"}},
	"chown -R":      {Level: SecurityLevelDangerous, Description: "递归修改所有者", Reasons: []string{"可能破坏系统权限"}},
	"mv /":          {Level: SecurityLevelDangerous, Description: "移动根目录", Reasons: []string{"会破坏系统结构"}},
	"kill -9 -1":    {Level: SecurityLevelDangerous, Description: "杀死所有进程", Reasons: []string{"会导致系统崩溃"}},
	"killall9":     {Level: SecurityLevelDangerous, Description: "杀死所有进程", Reasons: []string{"会导致系统崩溃"}},
}

// 需要特别审查的命令模式
var riskyCommandPatterns = []struct {
	Pattern     string
	Level       CommandSecurityLevel
	Description string
	Reason      string
}{
	{"rm -rf", SecurityLevelDangerous, "强制递归删除", "可能意外删除重要文件"},
	{"dd if=", SecurityLevelDangerous, "dd 磁盘写入命令", "可能覆盖重要数据"},
	{"mv /", SecurityLevelDangerous, "移动根目录", "会破坏系统结构"},
	{"chmod 777", SecurityLevelModerate, "设置全局可写权限", "安全风险"},
	{"chmod -R 777", SecurityLevelDangerous, "递归设置全局可写权限", "严重安全风险"},
	{"wget", SecurityLevelModerate, "下载文件", "可能下载恶意内容"},
	{"curl", SecurityLevelModerate, "下载/上传数据", "可能泄露数据或下载恶意内容"},
	{"> /", SecurityLevelDangerous, "重定向到系统目录", "可能破坏系统文件"},
	{"kill -9", SecurityLevelModerate, "强制终止进程", "可能导致数据丢失"},
	{"killall", SecurityLevelModerate, "终止进程组", "可能导致服务中断"},
	{"pkill", SecurityLevelModerate, "终止进程", "可能导致服务中断"},
}

// 命令执行历史记录
var commandHistory []CommandExecutionRecord

// CommandExecutionRecord 命令执行记录
type CommandExecutionRecord struct {
	Command      string
	ExecutedAt   time.Time
	ExitCode     int
	Duration     time.Duration
	SecurityLevel CommandSecurityLevel
	Approved     bool
}

// ExecuteCommandRequest 执行命令请求
type ExecuteCommandRequest struct {
	Command string `json:"command" jsonschema:"description=要执行的命令,required"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=超时时间（秒），默认30秒，最大300秒"`
}

// ExecuteCommandResponse 执行命令响应
type ExecuteCommandResponse struct {
	Stdout         string `json:"stdout" jsonschema:"description=标准输出内容"`
	Stderr         string `json:"stderr" jsonschema:"description=标准错误内容"`
	ExitCode       int    `json:"exit_code" jsonschema:"description=退出码，0表示成功"`
	ExecutionTime  string `json:"execution_time" jsonschema:"description=执行时长"`
	SecurityLevel  string `json:"security_level" jsonschema:"description=命令的安全级别"`
	WarningMessage string `json:"warning_message,omitempty" jsonschema:"description=警告信息"`
	ErrorMessage   string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// evaluateCommandSecurity 评估命令的安全级别
func evaluateCommandSecurity(command string) CommandDangerousLevel {
	cmdLower := strings.ToLower(command)

	// 检查是否在黑名单中
	for dangerousCmd, level := range dangerousCommands {
		if strings.Contains(cmdLower, dangerousCmd) {
			return level
		}
	}

	// 检查风险模式
	for _, pattern := range riskyCommandPatterns {
		if strings.Contains(cmdLower, pattern.Pattern) {
			return CommandDangerousLevel{
				Level:       pattern.Level,
				Description: pattern.Description,
				Reasons:     []string{pattern.Reason},
			}
		}
	}

	// 默认为中等风险
	return CommandDangerousLevel{
		Level:       SecurityLevelModerate,
		Description: "常规命令",
		Reasons:     []string{"需要监控执行"},
	}
}

// ExecuteCommand 执行 shell 命令
func ExecuteCommand(ctx context.Context, req *ExecuteCommandRequest) (*ExecuteCommandResponse, error) {
	if req.Command == "" {
		return &ExecuteCommandResponse{
			ErrorMessage: "command 参数是必需的",
		}, nil
	}

	// 评估命令安全性
	securityLevel := evaluateCommandSecurity(req.Command)

	// 如果是危险命令，拒绝执行
	if securityLevel.Level == SecurityLevelDangerous {
		return &ExecuteCommandResponse{
			ErrorMessage: fmt.Sprintf("命令执行被拒绝：检测到危险命令\n\n危险等级：%s\n风险描述：%s\n拒绝原因：%s\n\n出于安全考虑，此命令不会被执行。如需执行，请联系系统管理员。",
				getSecurityLevelName(securityLevel.Level),
				securityLevel.Description,
				strings.Join(securityLevel.Reasons, "；")),
			SecurityLevel: getSecurityLevelName(securityLevel.Level),
		}, nil
	}

	// 设置超时时间
	timeout := 30 * time.Second
	if req.Timeout > 0 && req.Timeout <= 300 {
		timeout = time.Duration(req.Timeout) * time.Second
	} else if req.Timeout > 300 {
		return &ExecuteCommandResponse{
			ErrorMessage: "超时时间不能超过 300 秒",
		}, nil
	}

	// 获取系统信息
	osType := runtime.GOOS
	var shell string
	var shellArgs []string

	switch osType {
	case "windows":
		shell = "cmd"
		shellArgs = []string{"/c", req.Command}
	case "darwin", "linux":
		shell = "/bin/bash"
		shellArgs = []string{"-c", req.Command}
	default:
		shell = "/bin/sh"
		shellArgs = []string{"-c", req.Command}
	}

	// 创建命令
	cmd := exec.CommandContext(ctx, shell, shellArgs...)

	// 设置进程组，便于控制子进程
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// 执行命令
	startTime := time.Now()
	stdout, stderr, exitCode := runCommandWithTimeout(cmd, timeout)
	executionTime := time.Since(startTime)

	// 记录命令执行历史
	record := CommandExecutionRecord{
		Command:       req.Command,
		ExecutedAt:    startTime,
		ExitCode:      exitCode,
		Duration:      executionTime,
		SecurityLevel: securityLevel.Level,
		Approved:      securityLevel.Level != SecurityLevelDangerous,
	}
	commandHistory = append(commandHistory, record)

	// 限制历史记录数量
	if len(commandHistory) > 1000 {
		commandHistory = commandHistory[len(commandHistory)-1000:]
	}

	// 构建响应
	response := &ExecuteCommandResponse{
		Stdout:        stdout,
		Stderr:        stderr,
		ExitCode:      exitCode,
		ExecutionTime: executionTime.String(),
		SecurityLevel: getSecurityLevelName(securityLevel.Level),
	}

	// 添加警告信息
	if securityLevel.Level == SecurityLevelModerate {
		response.WarningMessage = fmt.Sprintf("注意：此命令被评估为中等风险 (%s)。原因：%s",
			securityLevel.Description,
			strings.Join(securityLevel.Reasons, "；"))
	}

	return response, nil
}

// runCommandWithTimeout 带超时执行命令
func runCommandWithTimeout(cmd *exec.Cmd, timeout time.Duration) (stdout, stderr string, exitCode int) {
	// 创建带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	// 获取输出
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", -1
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", -1
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return "", "", -1
	}

	// 读取输出
	stdoutBytes := make([]byte, 1024*1024) // 限制输出为 1MB
	stderrBytes := make([]byte, 1024*1024)
	n1, _ := stdoutPipe.Read(stdoutBytes)
	n2, _ := stderrPipe.Read(stderrBytes)

	// 等待命令完成
	err = cmd.Wait()

	exitCode = 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return string(stdoutBytes[:n1]), string(stderrBytes[:n2]), exitCode
}

// getSecurityLevelName 获取安全级别名称
func getSecurityLevelName(level CommandSecurityLevel) string {
	switch level {
	case SecurityLevelSafe:
		return "安全"
	case SecurityLevelModerate:
		return "中等风险"
	case SecurityLevelDangerous:
		return "危险"
	default:
		return "未知"
	}
}

// GetSystemInfoRequest 获取系统信息请求
type GetSystemInfoRequest struct {
	InfoType string `json:"info_type" jsonschema:"description=信息类型: os, shell, path, env, all。默认为 all"`
}

// GetSystemInfoResponse 获取系统信息响应
type GetSystemInfoResponse struct {
	OS          string            `json:"os" jsonschema:"description=操作系统类型"`
	Arch        string            `json:"arch" jsonschema:"description=系统架构"`
	Shell       string            `json:"shell" jsonschema:"description=默认 shell"`
	WorkingDir  string            `json:"working_dir" jsonschema:"description=当前工作目录"`
	Environment map[string]string `json:"environment,omitempty" jsonschema:"description=环境变量（仅当 info_type 为 env 或 all 时返回）"`
	ErrorMessage string           `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetSystemInfo 获取系统信息
func GetSystemInfo(ctx context.Context, req *GetSystemInfoRequest) (*GetSystemInfoResponse, error) {
	infoType := "all"
	if req.InfoType != "" {
		infoType = req.InfoType
	}

	response := &GetSystemInfoResponse{
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		WorkingDir: getWorkingDir(),
	}

	// 设置 shell
	switch runtime.GOOS {
	case "windows":
		response.Shell = "cmd.exe (Windows Command Prompt)"
	case "darwin":
		response.Shell = "/bin/bash (Bash) or /bin/zsh (Z shell)"
	case "linux":
		response.Shell = "/bin/bash (Bash)"
	default:
		response.Shell = "/bin/sh (POSIX shell)"
	}

	// 如果请求环境变量
	if infoType == "env" || infoType == "all" {
		response.Environment = make(map[string]string)
		// 只返回安全的环境变量
		safeEnvVars := []string{
			"PATH", "HOME", "USER", "SHELL", "PWD", "LANG",
			"TERM", "GOPATH", "GOROOT", "NODE_ENV",
		}
		for _, key := range safeEnvVars {
			if val := os.Getenv(key); val != "" {
				response.Environment[key] = val
			}
		}
	}

	return response, nil
}

// getWorkingDir 获取当前工作目录
func getWorkingDir() string {
	if pwd, err := os.Getwd(); err == nil {
		return pwd
	}
	return "无法获取"
}

// GetCommandHistoryRequest 获取命令历史请求
type GetCommandHistoryRequest struct {
	Limit int `json:"limit,omitempty" jsonschema:"description=返回的历史记录数量，默认10条，最多100条"`
}

// GetCommandHistoryResponse 获取命令历史响应
type GetCommandHistoryResponse struct {
	History       []CommandExecutionRecord `json:"history" jsonschema:"description=命令执行历史"`
	TotalExecuted int                      `json:"total_executed" jsonschema:"description=总共执行的命令数量"`
	ErrorMessage  string                   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetCommandHistory 获取命令执行历史
func GetCommandHistory(ctx context.Context, req *GetCommandHistoryRequest) (*GetCommandHistoryResponse, error) {
	limit := 10
	if req.Limit > 0 && req.Limit <= 100 {
		limit = req.Limit
	}

	historyLen := len(commandHistory)
	start := 0
	if historyLen > limit {
		start = historyLen - limit
	}

	return &GetCommandHistoryResponse{
		History:       commandHistory[start:],
		TotalExecuted: historyLen,
	}, nil
}

// GetCLITools 获取所有 CLI 操作工具
func GetCLITools() ([]tool.BaseTool, error) {
	var tools []tool.BaseTool

	// 执行命令工具
	executeTool, err := utils.InferTool("execute_command", "执行 shell 命令。会根据操作系统自动选择合适的 shell（Windows 使用 cmd，macOS/Linux 使用 bash）。命令执行前会进行安全检查，拒绝执行危险命令。支持设置超时时间（默认30秒，最大300秒）", ExecuteCommand)
	if err != nil {
		return nil, err
	}
	tools = append(tools, executeTool)

	// 获取系统信息工具
	systemInfoTool, err := utils.InferTool("get_system_info", "获取系统信息，包括操作系统类型、架构、shell、工作目录、环境变量等。支持通过 info_type 参数指定返回的信息类型（os, shell, path, env, all）", GetSystemInfo)
	if err != nil {
		return nil, err
	}
	tools = append(tools, systemInfoTool)

	// 获取命令历史工具
	historyTool, err := utils.InferTool("get_command_history", "获取命令执行历史记录。可以查看之前执行过的命令、执行时间、退出码、安全级别等信息。支持通过 limit 参数限制返回的记录数量", GetCommandHistory)
	if err != nil {
		return nil, err
	}
	tools = append(tools, historyTool)

	return tools, nil
}

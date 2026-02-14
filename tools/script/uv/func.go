package uv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// UVTools 基于 uv 的 Python 脚本执行工具实例
type UVTools struct {
	// workDir 是工作目录，存放虚拟环境和脚本
	workDir string
	// venvPath 是虚拟环境的路径
	venvPath string
	// uvPath 是 uv 命令的路径
	uvPath string
}

// NewUVTools 创建一个新的 uv 工具实例
// workDir 是工作目录，用于存放虚拟环境和脚本
func NewUVTools(workDir string) (*UVTools, error) {
	// 转换为绝对路径
	absPath, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	// 检查目录是否存在，如果不存在则创建
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
		}
	}

	// 检查 uv 是否已安装
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("未找到 uv 命令，请先安装 uv: https://github.com/astral-sh/uv （可以使用 ./fkteams --init 或 ./fkteams -i 安装）")
	}

	venvPath := filepath.Join(absPath, ".venv")

	return &UVTools{
		workDir:  absPath,
		venvPath: venvPath,
		uvPath:   uvPath,
	}, nil
}

// executeCommand 执行命令并返回输出
func (ut *UVTools) executeCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = ut.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("命令执行失败: %w, 输出: %s", err, string(output))
	}

	return string(output), nil
}

// InitEnvRequest 初始化环境请求
type InitEnvRequest struct {
	PythonVersion string `json:"python_version,omitempty" jsonschema:"description=Python版本,例如3.11或3.12,不填则使用系统默认版本"`
	Force         bool   `json:"force,omitempty" jsonschema:"description=是否强制重新创建环境(删除已有环境)"`
}

// InitEnvResponse 初始化环境响应
type InitEnvResponse struct {
	Success      bool   `json:"success" jsonschema:"description=是否成功"`
	Message      string `json:"message" jsonschema:"description=执行信息"`
	VenvPath     string `json:"venv_path,omitempty" jsonschema:"description=虚拟环境路径"`
	PythonPath   string `json:"python_path,omitempty" jsonschema:"description=Python解释器路径"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// InitEnv 初始化 Python 虚拟环境
func (ut *UVTools) InitEnv(ctx context.Context, req *InitEnvRequest) (*InitEnvResponse, error) {
	// 检查是否已存在虚拟环境
	if _, err := os.Stat(ut.venvPath); err == nil {
		if !req.Force {
			pythonPath := ut.getPythonPath()
			return &InitEnvResponse{
				Success:    true,
				Message:    "虚拟环境已存在，无需重新创建。如需重新创建，请设置 force=true",
				VenvPath:   ut.venvPath,
				PythonPath: pythonPath,
			}, nil
		}
		// 强制模式：删除已有环境
		if err := os.RemoveAll(ut.venvPath); err != nil {
			return &InitEnvResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("删除已有环境失败: %v", err),
			}, nil
		}
	}

	// 构建命令
	args := []string{"venv", ut.venvPath}
	if req.PythonVersion != "" {
		args = append(args, "--python", req.PythonVersion)
	}

	// 创建虚拟环境
	output, err := ut.executeCommand(ctx, ut.uvPath, args...)
	if err != nil {
		return &InitEnvResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("创建虚拟环境失败: %v", err),
		}, nil
	}

	pythonPath := ut.getPythonPath()

	return &InitEnvResponse{
		Success:    true,
		Message:    fmt.Sprintf("虚拟环境创建成功\n%s", output),
		VenvPath:   ut.venvPath,
		PythonPath: pythonPath,
	}, nil
}

// getPythonPath 获取虚拟环境中的 Python 解释器路径
func (ut *UVTools) getPythonPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(ut.venvPath, "Scripts", "python.exe")
	}
	return filepath.Join(ut.venvPath, "bin", "python")
}

// InstallPackageRequest 安装依赖包请求
type InstallPackageRequest struct {
	Packages []string `json:"packages" jsonschema:"description=要安装的包列表,必填,required"`
	Upgrade  bool     `json:"upgrade,omitempty" jsonschema:"description=是否升级已安装的包"`
}

// InstallPackageResponse 安装依赖包响应
type InstallPackageResponse struct {
	Success      bool     `json:"success" jsonschema:"description=是否成功"`
	Message      string   `json:"message" jsonschema:"description=执行信息"`
	Installed    []string `json:"installed,omitempty" jsonschema:"description=已安装的包"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// InstallPackage 安装 Python 依赖包
func (ut *UVTools) InstallPackage(ctx context.Context, req *InstallPackageRequest) (*InstallPackageResponse, error) {
	if len(req.Packages) == 0 {
		return &InstallPackageResponse{
			Success:      false,
			ErrorMessage: "packages 不能为空",
		}, nil
	}

	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &InstallPackageResponse{
			Success:      false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	// 构建命令：uv pip install
	args := []string{"pip", "install"}
	if req.Upgrade {
		args = append(args, "--upgrade")
	}
	args = append(args, req.Packages...)

	// 执行安装
	output, err := ut.executeCommand(ctx, ut.uvPath, args...)
	if err != nil {
		return &InstallPackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("安装依赖失败: %v", err),
		}, nil
	}

	return &InstallPackageResponse{
		Success:   true,
		Message:   fmt.Sprintf("依赖安装成功\n%s", output),
		Installed: req.Packages,
	}, nil
}

// RemovePackageRequest 移除依赖包请求
type RemovePackageRequest struct {
	Packages []string `json:"packages" jsonschema:"description=要移除的包列表,必填,required"`
}

// RemovePackageResponse 移除依赖包响应
type RemovePackageResponse struct {
	Success      bool     `json:"success" jsonschema:"description=是否成功"`
	Message      string   `json:"message" jsonschema:"description=执行信息"`
	Removed      []string `json:"removed,omitempty" jsonschema:"description=已移除的包"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RemovePackage 移除 Python 依赖包
func (ut *UVTools) RemovePackage(ctx context.Context, req *RemovePackageRequest) (*RemovePackageResponse, error) {
	if len(req.Packages) == 0 {
		return &RemovePackageResponse{
			Success:      false,
			ErrorMessage: "packages 不能为空",
		}, nil
	}

	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &RemovePackageResponse{
			Success:      false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	// 构建命令：uv pip uninstall
	args := []string{"pip", "uninstall", "-y"}
	args = append(args, req.Packages...)

	// 执行卸载
	output, err := ut.executeCommand(ctx, ut.uvPath, args...)
	if err != nil {
		return &RemovePackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("移除依赖失败: %v", err),
		}, nil
	}

	return &RemovePackageResponse{
		Success: true,
		Message: fmt.Sprintf("依赖移除成功\n%s", output),
		Removed: req.Packages,
	}, nil
}

// ListPackageRequest 列出已安装包请求
type ListPackageRequest struct {
	Format string `json:"format,omitempty" jsonschema:"description=输出格式: json 或 text,默认为 json"`
}

// PackageInfo 包信息
type PackageInfo struct {
	Name    string `json:"name" jsonschema:"description=包名"`
	Version string `json:"version" jsonschema:"description=版本号"`
}

// ListPackageResponse 列出已安装包响应
type ListPackageResponse struct {
	Success      bool          `json:"success" jsonschema:"description=是否成功"`
	Packages     []PackageInfo `json:"packages,omitempty" jsonschema:"description=已安装的包列表"`
	Message      string        `json:"message,omitempty" jsonschema:"description=执行信息"`
	ErrorMessage string        `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// ListPackage 列出已安装的 Python 包
func (ut *UVTools) ListPackage(ctx context.Context, req *ListPackageRequest) (*ListPackageResponse, error) {
	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &ListPackageResponse{
			Success:      false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	// 构建命令：uv pip list
	args := []string{"pip", "list", "--format", "json"}

	// 执行列表
	output, err := ut.executeCommand(ctx, ut.uvPath, args...)
	if err != nil {
		return &ListPackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("列出依赖失败: %v", err),
		}, nil
	}

	// 解析 JSON 输出
	var packages []PackageInfo
	if err := json.Unmarshal([]byte(output), &packages); err != nil {
		return &ListPackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("解析输出失败: %v", err),
		}, nil
	}

	message := fmt.Sprintf("共有 %d 个已安装的包", len(packages))
	if req.Format == "text" {
		var lines []string
		for _, pkg := range packages {
			lines = append(lines, fmt.Sprintf("%s==%s", pkg.Name, pkg.Version))
		}
		message = strings.Join(lines, "\n")
	}

	return &ListPackageResponse{
		Success:  true,
		Packages: packages,
		Message:  message,
	}, nil
}

// CleanEnvRequest 清理环境请求
type CleanEnvRequest struct {
	KeepVenv bool `json:"keep_venv,omitempty" jsonschema:"description=是否保留虚拟环境目录(仅清理包)"`
}

// CleanEnvResponse 清理环境响应
type CleanEnvResponse struct {
	Success      bool   `json:"success" jsonschema:"description=是否成功"`
	Message      string `json:"message" jsonschema:"description=执行信息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CleanEnv 清理 Python 环境
func (ut *UVTools) CleanEnv(ctx context.Context, req *CleanEnvRequest) (*CleanEnvResponse, error) {
	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &CleanEnvResponse{
			Success: true,
			Message: "虚拟环境不存在，无需清理",
		}, nil
	}

	if req.KeepVenv {
		// 只清理包，不删除虚拟环境
		// 列出所有包
		listResp, err := ut.ListPackage(ctx, &ListPackageRequest{})
		if err != nil {
			return &CleanEnvResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("获取包列表失败: %v", err),
			}, nil
		}

		if len(listResp.Packages) == 0 {
			return &CleanEnvResponse{
				Success: true,
				Message: "环境中没有已安装的包",
			}, nil
		}

		// 卸载所有包
		var packageNames []string
		for _, pkg := range listResp.Packages {
			packageNames = append(packageNames, pkg.Name)
		}

		removeResp, err := ut.RemovePackage(ctx, &RemovePackageRequest{
			Packages: packageNames,
		})
		if err != nil {
			return &CleanEnvResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("清理包失败: %v", err),
			}, nil
		}

		return &CleanEnvResponse{
			Success: true,
			Message: fmt.Sprintf("环境清理成功，已移除 %d 个包\n%s", len(packageNames), removeResp.Message),
		}, nil
	}

	// 完全删除虚拟环境
	if err := os.RemoveAll(ut.venvPath); err != nil {
		return &CleanEnvResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("删除虚拟环境失败: %v", err),
		}, nil
	}

	return &CleanEnvResponse{
		Success: true,
		Message: "虚拟环境已完全删除",
	}, nil
}

// RunScriptRequest 运行脚本请求
type RunScriptRequest struct {
	ScriptPath    string   `json:"script_path,omitempty" jsonschema:"description=脚本文件路径,与 script_content 二选一"`
	ScriptContent string   `json:"script_content,omitempty" jsonschema:"description=脚本内容,与 script_path 二选一"`
	Args          []string `json:"args,omitempty" jsonschema:"description=传递给脚本的参数"`
	Timeout       int      `json:"timeout,omitempty" jsonschema:"description=超时时间(秒),默认300秒,最大600秒"`
}

// RunScriptResponse 运行脚本响应
type RunScriptResponse struct {
	Success      bool   `json:"success" jsonschema:"description=是否成功"`
	Output       string `json:"output,omitempty" jsonschema:"description=脚本输出"`
	ExitCode     int    `json:"exit_code,omitempty" jsonschema:"description=退出码"`
	Duration     string `json:"duration,omitempty" jsonschema:"description=执行时长"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RunScript 运行 Python 脚本
func (ut *UVTools) RunScript(ctx context.Context, req *RunScriptRequest) (*RunScriptResponse, error) {
	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &RunScriptResponse{
			Success:      false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	// 确定脚本路径
	var scriptPath string

	if req.ScriptPath != "" {
		// 使用提供的脚本路径
		if !filepath.IsAbs(req.ScriptPath) {
			scriptPath = filepath.Join(ut.workDir, req.ScriptPath)
		} else {
			scriptPath = req.ScriptPath
		}

		// 检查文件是否存在
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return &RunScriptResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("脚本文件不存在: %s，请检查文件路径或使用 file_list 查看可用文件", scriptPath),
			}, nil
		}
	} else if req.ScriptContent != "" {
		// 创建临时脚本文件
		tempFile, err := os.CreateTemp(ut.workDir, "script_*.py")
		if err != nil {
			return &RunScriptResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("创建临时脚本失败: %v", err),
			}, nil
		}
		scriptPath = tempFile.Name()

		// 写入脚本内容
		if _, err := tempFile.WriteString(req.ScriptContent); err != nil {
			tempFile.Close()
			os.Remove(scriptPath)
			return &RunScriptResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("写入脚本内容失败: %v", err),
			}, nil
		}
		tempFile.Close()

		// 如果是临时文件，执行完后删除
		defer os.Remove(scriptPath)
	} else {
		return &RunScriptResponse{
			Success:      false,
			ErrorMessage: "script_path 和 script_content 必须提供其中一个",
		}, nil
	}

	// 设置超时
	timeout := 300 * time.Second // 默认 300 秒
	if req.Timeout > 0 {
		if req.Timeout > 600 {
			req.Timeout = 600 // 最大 600 秒
		}
		timeout = time.Duration(req.Timeout) * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 构建命令：uv run
	pythonPath := ut.getPythonPath()
	args := []string{scriptPath}
	if len(req.Args) > 0 {
		args = append(args, req.Args...)
	}

	// 执行脚本
	startTime := time.Now()
	cmd := exec.CommandContext(execCtx, pythonPath, args...)
	cmd.Dir = ut.workDir

	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &RunScriptResponse{
				Success:      false,
				Output:       string(output),
				Duration:     duration.String(),
				ErrorMessage: fmt.Sprintf("脚本执行失败: %v", err),
			}, nil
		}
	}

	success := exitCode == 0

	message := ""
	if !success {
		message = fmt.Sprintf("脚本执行失败，退出码: %d", exitCode)
	}

	return &RunScriptResponse{
		Success:      success,
		Output:       string(output),
		ExitCode:     exitCode,
		Duration:     duration.String(),
		ErrorMessage: message,
	}, nil
}

// GetEnvInfoRequest 获取环境信息请求
type GetEnvInfoRequest struct{}

// GetEnvInfoResponse 获取环境信息响应
type GetEnvInfoResponse struct {
	Success       bool   `json:"success" jsonschema:"description=是否成功"`
	Exists        bool   `json:"exists" jsonschema:"description=虚拟环境是否存在"`
	VenvPath      string `json:"venv_path,omitempty" jsonschema:"description=虚拟环境路径"`
	PythonPath    string `json:"python_path,omitempty" jsonschema:"description=Python解释器路径"`
	PythonVersion string `json:"python_version,omitempty" jsonschema:"description=Python版本"`
	UVVersion     string `json:"uv_version,omitempty" jsonschema:"description=uv版本"`
	PackageCount  int    `json:"package_count,omitempty" jsonschema:"description=已安装包数量"`
	ErrorMessage  string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetEnvInfo 获取环境信息
func (ut *UVTools) GetEnvInfo(ctx context.Context, req *GetEnvInfoRequest) (*GetEnvInfoResponse, error) {
	resp := &GetEnvInfoResponse{
		Success:  true,
		VenvPath: ut.venvPath,
	}

	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		resp.Exists = false
		resp.ErrorMessage = "虚拟环境不存在，请先调用 init_env 初始化环境"
		return resp, nil
	}
	resp.Exists = true

	// 获取 Python 路径和版本
	pythonPath := ut.getPythonPath()
	resp.PythonPath = pythonPath

	// 获取 Python 版本
	if output, err := ut.executeCommand(ctx, pythonPath, "--version"); err == nil {
		resp.PythonVersion = strings.TrimSpace(output)
	}

	// 获取 uv 版本
	if output, err := ut.executeCommand(ctx, ut.uvPath, "--version"); err == nil {
		resp.UVVersion = strings.TrimSpace(output)
	}

	// 获取已安装包数量
	if listResp, err := ut.ListPackage(ctx, &ListPackageRequest{}); err == nil {
		resp.PackageCount = len(listResp.Packages)
	}

	return resp, nil
}

// RunCodeRequest 运行代码片段请求
type RunCodeRequest struct {
	Code    string   `json:"code" jsonschema:"description=要执行的 Python 代码,required"`
	Args    []string `json:"args,omitempty" jsonschema:"description=传递给代码的命令行参数"`
	Timeout int      `json:"timeout,omitempty" jsonschema:"description=超时时间(秒),默认60秒,最大300秒"`
}

// RunCodeResponse 运行代码片段响应
type RunCodeResponse struct {
	Success      bool   `json:"success" jsonschema:"description=是否成功"`
	Output       string `json:"output,omitempty" jsonschema:"description=代码输出"`
	ExitCode     int    `json:"exit_code,omitempty" jsonschema:"description=退出码"`
	Duration     string `json:"duration,omitempty" jsonschema:"description=执行时长"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RunCode 快速运行 Python 代码片段(无需创建文件)
func (ut *UVTools) RunCode(ctx context.Context, req *RunCodeRequest) (*RunCodeResponse, error) {
	if req.Code == "" {
		return &RunCodeResponse{
			Success:      false,
			ErrorMessage: "code 参数是必需的",
		}, nil
	}

	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &RunCodeResponse{
			Success:      false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	// 设置超时
	timeout := 60 * time.Second // 默认 60 秒
	if req.Timeout > 0 {
		if req.Timeout > 300 {
			req.Timeout = 300 // 最大 300 秒
		}
		timeout = time.Duration(req.Timeout) * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 构建命令：python -c "code"
	pythonPath := ut.getPythonPath()
	args := []string{"-c", req.Code}
	if len(req.Args) > 0 {
		args = append(args, req.Args...)
	}

	// 执行代码
	startTime := time.Now()
	cmd := exec.CommandContext(execCtx, pythonPath, args...)
	cmd.Dir = ut.workDir

	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &RunCodeResponse{
				Success:      false,
				Output:       string(output),
				Duration:     duration.String(),
				ErrorMessage: fmt.Sprintf("代码执行失败: %v", err),
			}, nil
		}
	}

	success := exitCode == 0

	message := ""
	if !success {
		message = fmt.Sprintf("代码执行失败，退出码: %d", exitCode)
	}

	return &RunCodeResponse{
		Success:      success,
		Output:       string(output),
		ExitCode:     exitCode,
		Duration:     duration.String(),
		ErrorMessage: message,
	}, nil
}

// CheckSyntaxRequest 检查 Python 代码语法请求
type CheckSyntaxRequest struct {
	Code     string `json:"code,omitempty" jsonschema:"description=要检查的 Python 代码,与 filepath 二选一"`
	Filepath string `json:"filepath,omitempty" jsonschema:"description=要检查的脚本文件路径,与 code 二选一"`
}

// SyntaxError 语法错误详情
type SyntaxError struct {
	Line    int    `json:"line" jsonschema:"description=错误行号"`
	Offset  int    `json:"offset" jsonschema:"description=错误列位置"`
	Message string `json:"message" jsonschema:"description=错误信息"`
	Text    string `json:"text,omitempty" jsonschema:"description=错误代码行"`
}

// CheckSyntaxResponse 检查语法响应
type CheckSyntaxResponse struct {
	Valid        bool          `json:"valid" jsonschema:"description=代码语法是否正确"`
	Errors       []SyntaxError `json:"errors,omitempty" jsonschema:"description=语法错误列表"`
	ErrorMessage string        `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CheckSyntax 检查 Python 代码语法
func (ut *UVTools) CheckSyntax(ctx context.Context, req *CheckSyntaxRequest) (*CheckSyntaxResponse, error) {
	if req.Code == "" && req.Filepath == "" {
		return &CheckSyntaxResponse{
			Valid:        false,
			ErrorMessage: "code 和 filepath 必须提供其中一个",
		}, nil
	}

	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &CheckSyntaxResponse{
			Valid:        false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	pythonPath := ut.getPythonPath()

	// 构建检查脚本
	checkScript := `
import sys
import ast
import traceback

code_or_file = sys.argv[1]
is_file = sys.argv[2] == 'file'

try:
    if is_file:
        with open(code_or_file, 'r', encoding='utf-8') as f:
            code = f.read()
    else:
        code = code_or_file
    
    ast.parse(code)
    print("SYNTAX_OK")
except SyntaxError as e:
    print(f"SYNTAX_ERROR:{e.lineno}:{e.offset}:{e.msg}:{e.text if e.text else ''}")
except Exception as e:
    print(f"ERROR:{str(e)}")
`

	var args []string
	if req.Filepath != "" {
		// 检查文件
		scriptPath := req.Filepath
		if !filepath.IsAbs(scriptPath) {
			scriptPath = filepath.Join(ut.workDir, scriptPath)
		}
		args = []string{"-c", checkScript, scriptPath, "file"}
	} else {
		// 检查代码
		args = []string{"-c", checkScript, req.Code, "code"}
	}

	cmd := exec.CommandContext(ctx, pythonPath, args...)
	cmd.Dir = ut.workDir

	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))

	if err != nil {
		return &CheckSyntaxResponse{
			Valid:        false,
			ErrorMessage: fmt.Sprintf("语法检查失败: %v, 输出: %s", err, result),
		}, nil
	}

	if result == "SYNTAX_OK" {
		return &CheckSyntaxResponse{
			Valid: true,
		}, nil
	}

	// 解析语法错误
	var errors []SyntaxError
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "SYNTAX_ERROR:") {
			parts := strings.SplitN(line[13:], ":", 4)
			if len(parts) >= 3 {
				lineNum, _ := strconv.Atoi(parts[0])
				offset, _ := strconv.Atoi(parts[1])
				message := parts[2]
				text := ""
				if len(parts) >= 4 {
					text = parts[3]
				}
				errors = append(errors, SyntaxError{
					Line:    lineNum,
					Offset:  offset,
					Message: message,
					Text:    text,
				})
			}
		} else if strings.HasPrefix(line, "ERROR:") {
			errors = append(errors, SyntaxError{
				Line:    0,
				Message: line[6:],
			})
		}
	}

	return &CheckSyntaxResponse{
		Valid:  false,
		Errors: errors,
	}, nil
}

// FormatCodeRequest 格式化 Python 代码请求
type FormatCodeRequest struct {
	Code string `json:"code" jsonschema:"description=要格式化的 Python 代码,required"`
}

// FormatCodeResponse 格式化代码响应
type FormatCodeResponse struct {
	Success       bool   `json:"success" jsonschema:"description=是否成功"`
	FormattedCode string `json:"formatted_code,omitempty" jsonschema:"description=格式化后的代码"`
	ErrorMessage  string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// FormatCode 格式化 Python 代码 (使用 autopep8)
func (ut *UVTools) FormatCode(ctx context.Context, req *FormatCodeRequest) (*FormatCodeResponse, error) {
	if req.Code == "" {
		return &FormatCodeResponse{
			Success:      false,
			ErrorMessage: "code 参数是必需的",
		}, nil
	}

	// 检查虚拟环境是否存在
	if _, err := os.Stat(ut.venvPath); os.IsNotExist(err) {
		return &FormatCodeResponse{
			Success:      false,
			ErrorMessage: "虚拟环境不存在，请先调用 uv_init_env 初始化环境",
		}, nil
	}

	pythonPath := ut.getPythonPath()

	// 尝试使用 autopep8 格式化
	formatScript := `
import sys
try:
    import autopep8
    code = sys.argv[1]
    formatted = autopep8.fix_code(code)
    print(formatted, end='')
except ImportError:
    # autopep8 未安装，返回原代码
    print(sys.argv[1], end='')
except Exception as e:
    print(f"ERROR:{str(e)}", file=sys.stderr)
    sys.exit(1)
`

	args := []string{"-c", formatScript, req.Code}
	cmd := exec.CommandContext(ctx, pythonPath, args...)
	cmd.Dir = ut.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &FormatCodeResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("代码格式化失败: %v", err),
		}, nil
	}

	return &FormatCodeResponse{
		Success:       true,
		FormattedCode: string(output),
	}, nil
}

package bun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BunTools 基于 bun 的 JavaScript 脚本执行工具实例
type BunTools struct {
	// workDir 是工作目录，存放项目和脚本
	workDir string
	// bunPath 是 bun 命令的路径
	bunPath string
}

// NewBunTools 创建一个新的 bun 工具实例
// workDir 是工作目录，用于存放项目和脚本
func NewBunTools(workDir string) (*BunTools, error) {
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

	// 检查 bun 是否已安装
	bunPath, err := exec.LookPath("bun")
	if err != nil {
		return nil, fmt.Errorf("未找到 bun 命令，请先安装 bun: https://bun.sh")
	}

	return &BunTools{
		workDir: absPath,
		bunPath: bunPath,
	}, nil
}

// executeCommand 执行命令并返回输出
func (bt *BunTools) executeCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = bt.workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("命令执行失败: %w, 输出: %s", err, string(output))
	}

	return string(output), nil
}

// InitEnvRequest 初始化环境请求
type InitEnvRequest struct {
	Force bool `json:"force,omitempty" jsonschema:"description=是否强制重新创建环境(删除已有package.json)"`
}

// InitEnvResponse 初始化环境响应
type InitEnvResponse struct {
	Success      bool   `json:"success" jsonschema:"description=是否成功"`
	Message      string `json:"message" jsonschema:"description=执行信息"`
	PackageJSON  string `json:"package_json,omitempty" jsonschema:"description=package.json路径"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// InitEnv 初始化 JavaScript 项目环境
func (bt *BunTools) InitEnv(ctx context.Context, req *InitEnvRequest) (*InitEnvResponse, error) {
	packageJSONPath := filepath.Join(bt.workDir, "package.json")

	// 检查是否已存在 package.json
	if _, err := os.Stat(packageJSONPath); err == nil {
		if !req.Force {
			return &InitEnvResponse{
				Success:     true,
				Message:     "项目已初始化，package.json 已存在。如需重新初始化，请设置 force=true",
				PackageJSON: packageJSONPath,
			}, nil
		}
		// 强制模式：删除已有的 package.json
		if err := os.Remove(packageJSONPath); err != nil {
			return &InitEnvResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("删除已有 package.json 失败: %v", err),
			}, fmt.Errorf("删除已有 package.json 失败: %w", err)
		}
	}

	// 使用 bun init 初始化项目
	output, err := bt.executeCommand(ctx, bt.bunPath, "init", "-y")
	if err != nil {
		return &InitEnvResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("初始化项目失败: %v", err),
		}, err
	}

	return &InitEnvResponse{
		Success:     true,
		Message:     fmt.Sprintf("项目初始化成功\n%s", output),
		PackageJSON: packageJSONPath,
	}, nil
}

// InstallPackageRequest 安装依赖包请求
type InstallPackageRequest struct {
	Packages []string `json:"packages" jsonschema:"description=要安装的包列表,必填,required"`
	Dev      bool     `json:"dev,omitempty" jsonschema:"description=是否作为开发依赖安装"`
	Global   bool     `json:"global,omitempty" jsonschema:"description=是否全局安装"`
}

// InstallPackageResponse 安装依赖包响应
type InstallPackageResponse struct {
	Success      bool     `json:"success" jsonschema:"description=是否成功"`
	Message      string   `json:"message" jsonschema:"description=执行信息"`
	Installed    []string `json:"installed,omitempty" jsonschema:"description=已安装的包"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// InstallPackage 安装 JavaScript 依赖包
func (bt *BunTools) InstallPackage(ctx context.Context, req *InstallPackageRequest) (*InstallPackageResponse, error) {
	if len(req.Packages) == 0 {
		return &InstallPackageResponse{
			Success:      false,
			ErrorMessage: "packages 不能为空",
		}, fmt.Errorf("packages 不能为空")
	}

	// 如果不是全局安装，检查 package.json 是否存在
	if !req.Global {
		packageJSONPath := filepath.Join(bt.workDir, "package.json")
		if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
			return &InstallPackageResponse{
				Success:      false,
				ErrorMessage: "项目未初始化，请先调用 init_env 初始化环境",
			}, fmt.Errorf("项目未初始化")
		}
	}

	// 构建命令：bun add 或 bun install -g
	args := []string{}
	if req.Global {
		args = append(args, "install", "-g")
	} else {
		args = append(args, "add")
		if req.Dev {
			args = append(args, "-d")
		}
	}
	args = append(args, req.Packages...)

	// 执行安装
	output, err := bt.executeCommand(ctx, bt.bunPath, args...)
	if err != nil {
		return &InstallPackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("安装依赖失败: %v", err),
		}, err
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
	Global   bool     `json:"global,omitempty" jsonschema:"description=是否从全局移除"`
}

// RemovePackageResponse 移除依赖包响应
type RemovePackageResponse struct {
	Success      bool     `json:"success" jsonschema:"description=是否成功"`
	Message      string   `json:"message" jsonschema:"description=执行信息"`
	Removed      []string `json:"removed,omitempty" jsonschema:"description=已移除的包"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RemovePackage 移除 JavaScript 依赖包
func (bt *BunTools) RemovePackage(ctx context.Context, req *RemovePackageRequest) (*RemovePackageResponse, error) {
	if len(req.Packages) == 0 {
		return &RemovePackageResponse{
			Success:      false,
			ErrorMessage: "packages 不能为空",
		}, fmt.Errorf("packages 不能为空")
	}

	// 如果不是全局删除，检查 package.json 是否存在
	if !req.Global {
		packageJSONPath := filepath.Join(bt.workDir, "package.json")
		if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
			return &RemovePackageResponse{
				Success:      false,
				ErrorMessage: "项目未初始化，请先调用 init_env 初始化环境",
			}, fmt.Errorf("项目未初始化")
		}
	}

	// 构建命令：bun remove
	args := []string{"remove"}
	if req.Global {
		args = append(args, "-g")
	}
	args = append(args, req.Packages...)

	// 执行卸载
	output, err := bt.executeCommand(ctx, bt.bunPath, args...)
	if err != nil {
		return &RemovePackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("移除依赖失败: %v", err),
		}, err
	}

	return &RemovePackageResponse{
		Success: true,
		Message: fmt.Sprintf("依赖移除成功\n%s", output),
		Removed: req.Packages,
	}, nil
}

// ListPackageRequest 列出已安装包请求
type ListPackageRequest struct {
	Global bool `json:"global,omitempty" jsonschema:"description=是否列出全局包"`
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

// ListPackage 列出已安装的 JavaScript 包
func (bt *BunTools) ListPackage(ctx context.Context, req *ListPackageRequest) (*ListPackageResponse, error) {
	// 如果不是全局列表，检查 package.json 是否存在
	if !req.Global {
		packageJSONPath := filepath.Join(bt.workDir, "package.json")
		if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
			return &ListPackageResponse{
				Success:      false,
				ErrorMessage: "项目未初始化，请先调用 init_env 初始化环境",
			}, fmt.Errorf("项目未初始化")
		}
	}

	// 读取 package.json 获取依赖列表
	packageJSONPath := filepath.Join(bt.workDir, "package.json")
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return &ListPackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("读取 package.json 失败: %v", err),
		}, err
	}

	var pkgJSON struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}

	if err := json.Unmarshal(data, &pkgJSON); err != nil {
		return &ListPackageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("解析 package.json 失败: %v", err),
		}, err
	}

	var packages []PackageInfo
	for name, version := range pkgJSON.Dependencies {
		packages = append(packages, PackageInfo{Name: name, Version: version})
	}
	for name, version := range pkgJSON.DevDependencies {
		packages = append(packages, PackageInfo{Name: name, Version: version + " (dev)"})
	}

	message := fmt.Sprintf("共有 %d 个已安装的包", len(packages))

	return &ListPackageResponse{
		Success:  true,
		Packages: packages,
		Message:  message,
	}, nil
}

// CleanEnvRequest 清理环境请求
type CleanEnvRequest struct {
	KeepPackageJSON bool `json:"keep_package_json,omitempty" jsonschema:"description=是否保留package.json(仅清理node_modules)"`
}

// CleanEnvResponse 清理环境响应
type CleanEnvResponse struct {
	Success      bool   `json:"success" jsonschema:"description=是否成功"`
	Message      string `json:"message" jsonschema:"description=执行信息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CleanEnv 清理 JavaScript 环境
func (bt *BunTools) CleanEnv(ctx context.Context, req *CleanEnvRequest) (*CleanEnvResponse, error) {
	nodeModulesPath := filepath.Join(bt.workDir, "node_modules")
	packageJSONPath := filepath.Join(bt.workDir, "package.json")
	bunLockPath := filepath.Join(bt.workDir, "bun.lockb")

	// 删除 node_modules
	if _, err := os.Stat(nodeModulesPath); err == nil {
		if err := os.RemoveAll(nodeModulesPath); err != nil {
			return &CleanEnvResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("删除 node_modules 失败: %v", err),
			}, err
		}
	}

	// 删除 bun.lockb
	if _, err := os.Stat(bunLockPath); err == nil {
		if err := os.Remove(bunLockPath); err != nil {
			return &CleanEnvResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("删除 bun.lockb 失败: %v", err),
			}, err
		}
	}

	message := "已清理 node_modules 和 bun.lockb"

	if !req.KeepPackageJSON {
		// 删除 package.json
		if _, err := os.Stat(packageJSONPath); err == nil {
			if err := os.Remove(packageJSONPath); err != nil {
				return &CleanEnvResponse{
					Success:      false,
					ErrorMessage: fmt.Sprintf("删除 package.json 失败: %v", err),
				}, err
			}
			message += "，已删除 package.json"
		}
	} else {
		message += "，保留了 package.json"
	}

	return &CleanEnvResponse{
		Success: true,
		Message: message,
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

// RunScript 运行 JavaScript 脚本
func (bt *BunTools) RunScript(ctx context.Context, req *RunScriptRequest) (*RunScriptResponse, error) {
	// 确定脚本路径
	var scriptPath string

	if req.ScriptPath != "" {
		// 使用提供的脚本路径
		if !filepath.IsAbs(req.ScriptPath) {
			scriptPath = filepath.Join(bt.workDir, req.ScriptPath)
		} else {
			scriptPath = req.ScriptPath
		}

		// 检查文件是否存在
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return &RunScriptResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("脚本文件不存在: %s", scriptPath),
			}, fmt.Errorf("脚本文件不存在: %s", scriptPath)
		}
	} else if req.ScriptContent != "" {
		// 创建临时脚本文件
		tempFile, err := os.CreateTemp(bt.workDir, "script_*.js")
		if err != nil {
			return &RunScriptResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("创建临时脚本失败: %v", err),
			}, err
		}
		scriptPath = tempFile.Name()

		// 写入脚本内容
		if _, err := tempFile.WriteString(req.ScriptContent); err != nil {
			tempFile.Close()
			os.Remove(scriptPath)
			return &RunScriptResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("写入脚本内容失败: %v", err),
			}, err
		}
		tempFile.Close()

		// 如果是临时文件，执行完后删除
		defer os.Remove(scriptPath)
	} else {
		return &RunScriptResponse{
			Success:      false,
			ErrorMessage: "script_path 和 script_content 必须提供其中一个",
		}, fmt.Errorf("script_path 和 script_content 必须提供其中一个")
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

	// 构建命令：bun run
	args := []string{"run", scriptPath}
	if len(req.Args) > 0 {
		args = append(args, req.Args...)
	}

	// 执行脚本
	startTime := time.Now()
	cmd := exec.CommandContext(execCtx, bt.bunPath, args...)
	cmd.Dir = bt.workDir

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
			}, err
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
	Success        bool   `json:"success" jsonschema:"description=是否成功"`
	Initialized    bool   `json:"initialized" jsonschema:"description=项目是否已初始化"`
	WorkDir        string `json:"work_dir,omitempty" jsonschema:"description=工作目录"`
	PackageJSON    string `json:"package_json,omitempty" jsonschema:"description=package.json路径"`
	BunVersion     string `json:"bun_version,omitempty" jsonschema:"description=bun版本"`
	PackageCount   int    `json:"package_count,omitempty" jsonschema:"description=已安装包数量"`
	HasNodeModules bool   `json:"has_node_modules,omitempty" jsonschema:"description=是否存在node_modules"`
	ErrorMessage   string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetEnvInfo 获取环境信息
func (bt *BunTools) GetEnvInfo(ctx context.Context, req *GetEnvInfoRequest) (*GetEnvInfoResponse, error) {
	resp := &GetEnvInfoResponse{
		Success: true,
		WorkDir: bt.workDir,
	}

	// 检查 package.json 是否存在
	packageJSONPath := filepath.Join(bt.workDir, "package.json")
	if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
		resp.Initialized = false
		resp.ErrorMessage = "项目未初始化，请先调用 init_env 初始化环境"
		return resp, nil
	}
	resp.Initialized = true
	resp.PackageJSON = packageJSONPath

	// 获取 bun 版本
	if output, err := bt.executeCommand(ctx, bt.bunPath, "--version"); err == nil {
		resp.BunVersion = strings.TrimSpace(output)
	}

	// 获取已安装包数量
	if listResp, err := bt.ListPackage(ctx, &ListPackageRequest{}); err == nil {
		resp.PackageCount = len(listResp.Packages)
	}

	// 检查 node_modules 是否存在
	nodeModulesPath := filepath.Join(bt.workDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); err == nil {
		resp.HasNodeModules = true
	}

	return resp, nil
}

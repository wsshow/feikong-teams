package uv

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
)

// GetTools 获取所有 Python 脚本工具
func (ut *UVTools) GetTools() ([]runtimeport.Tool, error) {
	if ut == nil {
		return nil, fmt.Errorf("uv 工具未初始化")
	}
	// 环境初始化工具

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "uv_init_env", Description: "初始化 Python 虚拟环境。使用 uv 创建隔离的 Python 环境，可以指定 Python 版本。如果 uv 尚未安装，可以通过运行 `fkteams init --env uv` 命令来安装和初始化 uv 环境。", Handler: ut.InitEnv},
		{Name: "uv_install_package", Description: "安装 Python 依赖包。支持一次安装多个包，使用 uv 的高速下载能力。", Handler: ut.InstallPackage},
		{Name: "uv_remove_package", Description: "移除已安装的 Python 依赖包。支持批量卸载。", Handler: ut.RemovePackage},
		{Name: "uv_list_package", Description: "列出虚拟环境中已安装的所有 Python 包及其版本。", Handler: ut.ListPackage},
		{Name: "uv_clean_env", Description: "清理 Python 环境。可以选择仅清理包或完全删除虚拟环境。", Handler: ut.CleanEnv},
		{Name: "uv_run_script", Description: "运行 Python 脚本。可以执行文件或直接运行代码内容，支持传递参数和设置超时。", Handler: ut.RunScript},
		{Name: "uv_get_env_info", Description: "获取 Python 虚拟环境的详细信息，包括环境路径、Python 版本、已安装包数量等。", Handler: ut.GetEnvInfo},
		{Name: "uv_run_code", Description: "快速执行 Python 代码片段(无需创建文件)。适用于测试小段代码、验证想法或快速计算。", Handler: ut.RunCode},
		{Name: "uv_check_syntax", Description: "检查 Python 代码或文件的语法错误。在执行前验证代码正确性，避免运行时错误。", Handler: ut.CheckSyntax},
		{Name: "uv_format_code", Description: "格式化 Python 代码，使其符合 PEP8 规范。提高代码可读性和一致性。", Handler: ut.FormatCode},
	})
}

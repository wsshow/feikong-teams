package uv

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有 Python 脚本工具
func (ut *UVTools) GetTools() ([]tool.BaseTool, error) {
	if ut == nil {
		return nil, fmt.Errorf("uv 工具未初始化")
	}

	var tools []tool.BaseTool

	// 环境初始化工具
	initEnvTool, err := utils.InferTool("uv_init_env", "初始化 Python 虚拟环境。使用 uv 创建隔离的 Python 环境，可以指定 Python 版本。", ut.InitEnv)
	if err != nil {
		return nil, err
	}
	tools = append(tools, initEnvTool)

	// 安装依赖包工具
	installPackageTool, err := utils.InferTool("uv_install_package", "安装 Python 依赖包。支持一次安装多个包，使用 uv 的高速下载能力。", ut.InstallPackage)
	if err != nil {
		return nil, err
	}
	tools = append(tools, installPackageTool)

	// 移除依赖包工具
	removePackageTool, err := utils.InferTool("uv_remove_package", "移除已安装的 Python 依赖包。支持批量卸载。", ut.RemovePackage)
	if err != nil {
		return nil, err
	}
	tools = append(tools, removePackageTool)

	// 列出已安装包工具
	listPackageTool, err := utils.InferTool("uv_list_package", "列出虚拟环境中已安装的所有 Python 包及其版本。", ut.ListPackage)
	if err != nil {
		return nil, err
	}
	tools = append(tools, listPackageTool)

	// 清理环境工具
	cleanEnvTool, err := utils.InferTool("uv_clean_env", "清理 Python 环境。可以选择仅清理包或完全删除虚拟环境。", ut.CleanEnv)
	if err != nil {
		return nil, err
	}
	tools = append(tools, cleanEnvTool)

	// 运行脚本工具
	runScriptTool, err := utils.InferTool("uv_run_script", "运行 Python 脚本。可以执行文件或直接运行代码内容，支持传递参数和设置超时。", ut.RunScript)
	if err != nil {
		return nil, err
	}
	tools = append(tools, runScriptTool)

	// 获取环境信息工具
	getEnvInfoTool, err := utils.InferTool("uv_get_env_info", "获取 Python 虚拟环境的详细信息，包括环境路径、Python 版本、已安装包数量等。", ut.GetEnvInfo)
	if err != nil {
		return nil, err
	}
	tools = append(tools, getEnvInfoTool)

	// 运行代码片段工具
	runCodeTool, err := utils.InferTool("uv_run_code", "快速执行 Python 代码片段(无需创建文件)。适用于测试小段代码、验证想法或快速计算。", ut.RunCode)
	if err != nil {
		return nil, err
	}
	tools = append(tools, runCodeTool)

	// 语法检查工具
	checkSyntaxTool, err := utils.InferTool("uv_check_syntax", "检查 Python 代码或文件的语法错误。在执行前验证代码正确性，避免运行时错误。", ut.CheckSyntax)
	if err != nil {
		return nil, err
	}
	tools = append(tools, checkSyntaxTool)

	// 代码格式化工具
	formatCodeTool, err := utils.InferTool("uv_format_code", "格式化 Python 代码，使其符合 PEP8 规范。提高代码可读性和一致性。", ut.FormatCode)
	if err != nil {
		return nil, err
	}
	tools = append(tools, formatCodeTool)

	return tools, nil
}

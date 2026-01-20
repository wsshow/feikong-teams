package bun

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有 JavaScript 脚本工具
func (bt *BunTools) GetTools() ([]tool.BaseTool, error) {
	if bt == nil {
		return nil, fmt.Errorf("bun 工具未初始化")
	}

	var tools []tool.BaseTool

	// 环境初始化工具
	initEnvTool, err := utils.InferTool("bun_init_env", "初始化 JavaScript 项目环境。使用 bun 创建项目配置文件 package.json。", bt.InitEnv)
	if err != nil {
		return nil, err
	}
	tools = append(tools, initEnvTool)

	// 安装依赖包工具
	installPackageTool, err := utils.InferTool("bun_install_package", "安装 JavaScript 依赖包。支持一次安装多个包，使用 bun 的极速安装能力。可以安装为开发依赖或全局安装。", bt.InstallPackage)
	if err != nil {
		return nil, err
	}
	tools = append(tools, installPackageTool)

	// 移除依赖包工具
	removePackageTool, err := utils.InferTool("bun_remove_package", "移除已安装的 JavaScript 依赖包。支持批量卸载和全局卸载。", bt.RemovePackage)
	if err != nil {
		return nil, err
	}
	tools = append(tools, removePackageTool)

	// 列出已安装包工具
	listPackageTool, err := utils.InferTool("bun_list_package", "列出项目中已安装的所有 JavaScript 包及其版本。", bt.ListPackage)
	if err != nil {
		return nil, err
	}
	tools = append(tools, listPackageTool)

	// 清理环境工具
	cleanEnvTool, err := utils.InferTool("bun_clean_env", "清理 JavaScript 环境。可以选择仅清理 node_modules 或完全清理项目文件。", bt.CleanEnv)
	if err != nil {
		return nil, err
	}
	tools = append(tools, cleanEnvTool)

	// 运行脚本工具
	runScriptTool, err := utils.InferTool("bun_run_script", "运行 JavaScript 脚本。可以执行文件或直接运行代码内容，支持传递参数和设置超时。", bt.RunScript)
	if err != nil {
		return nil, err
	}
	tools = append(tools, runScriptTool)

	// 获取环境信息工具
	getEnvInfoTool, err := utils.InferTool("bun_get_env_info", "获取 JavaScript 项目的详细信息，包括工作目录、bun 版本、已安装包数量等。", bt.GetEnvInfo)
	if err != nil {
		return nil, err
	}
	tools = append(tools, getEnvInfoTool)

	return tools, nil
}

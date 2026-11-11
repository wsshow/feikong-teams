package bun

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
)

// GetTools 获取所有 JavaScript 脚本工具
func (bt *BunTools) GetTools() ([]runtimeport.Tool, error) {
	if bt == nil {
		return nil, fmt.Errorf("bun 工具未初始化")
	}
	// 环境初始化工具

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "bun_init_env", Description: "初始化 JavaScript 项目环境。使用 bun 创建项目配置文件 package.json。如果 bun 尚未安装，可以通过运行 `fkteams init --env bun` 命令来安装和初始化 bun 环境。", Handler: bt.InitEnv},
		{Name: "bun_install_package", Description: "安装 JavaScript 依赖包。支持一次安装多个包，使用 bun 的极速安装能力。可以安装为开发依赖或全局安装。", Handler: bt.InstallPackage},
		{Name: "bun_remove_package", Description: "移除已安装的 JavaScript 依赖包。支持批量卸载和全局卸载。", Handler: bt.RemovePackage},
		{Name: "bun_list_package", Description: "列出项目中已安装的所有 JavaScript 包及其版本。", Handler: bt.ListPackage},
		{Name: "bun_clean_env", Description: "清理 JavaScript 环境。可以选择仅清理 node_modules 或完全清理项目文件。", Handler: bt.CleanEnv},
		{Name: "bun_run_script", Description: "运行 JavaScript 脚本。可以执行文件或直接运行代码内容，支持传递参数和设置超时。", Handler: bt.RunScript},
		{Name: "bun_get_env_info", Description: "获取 JavaScript 项目的详细信息，包括工作目录、bun 版本、已安装包数量等。", Handler: bt.GetEnvInfo},
	})
}

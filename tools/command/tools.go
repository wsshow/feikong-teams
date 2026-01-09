package command

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取所有 CLI 操作工具
func GetTools() ([]tool.BaseTool, error) {
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

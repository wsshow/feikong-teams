package command

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 返回命令行工具列表
func (t *CommandTools) GetTools() ([]tool.BaseTool, error) {
	smartExec, err := utils.InferTool(
		"smart_execute",
		"智能命令执行工具，带安全审批功能。可执行任意 shell 命令并自动评估安全风险。安全命令直接执行，危险命令暂停并请求用户审批。支持超时控制（默认60秒，最大600秒），输出限制 1MB。适合系统管理、文件操作、包管理、构建编译、数据处理等任务。使用时必须提供执行原因。",
		t.SmartExecute,
	)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{smartExec}, nil
}

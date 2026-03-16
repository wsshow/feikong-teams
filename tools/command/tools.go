package command

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 返回命令行工具列表
func (t *CommandTools) GetTools() ([]tool.BaseTool, error) {
	exec, err := utils.InferTool(
		"execute",
		"命令执行工具，带安全审批功能。可执行任意 shell 命令并自动评估安全风险。安全命令直接执行，危险命令暂停并请求用户审批。超时控制默认60秒，最大600秒。使用时必须提供执行原因。",
		t.SmartExecute,
	)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{exec}, nil
}

package command

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 返回命令行工具列表
func (t *CommandTools) GetTools() ([]tool.BaseTool, error) {
	exec, err := utils.InferTool(
		"execute",
		"命令执行工具，带安全审批功能。可执行任意 shell 命令并自动评估安全风险。安全命令直接执行，危险命令暂停并请求用户审批。超时控制默认60秒，最大600秒。使用时必须提供执行原因。"+
			"耗时超过15秒的命令会自动转入后台执行并返回 task_id。"+
			"输出超过1MB会被截断并标记 output_truncated，此时建议将输出重定向到文件。"+
			"对于需要持续运行的服务类命令（如 HTTP server、watch、tail -f 等），设置 background=true，命令会立即以 nohup 后台启动并返回 PID，通过 kill PID 终止。"+
			"后台任务管理: task_action=list 列出所有任务, task_action=status+task_id 查询状态, task_action=terminate+task_id 终止任务。",
		t.SmartExecute,
	)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{exec}, nil
}

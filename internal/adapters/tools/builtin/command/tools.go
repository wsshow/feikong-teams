package command

import runtimeport "fkteams/internal/ports/runtime"

// GetTools 返回命令执行和任务管理工具。
func (t *CommandTools) GetTools() ([]runtimeport.Tool, error) {
	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "execute", Description: "在工作区执行 shell 命令并进行安全审批。job 默认执行期限120秒，service 默认持续运行；timeout 指定执行期限。yield_ms 只控制本次等待预算，默认1000毫秒，耗尽后返回 task_id，后台归应用管理。background=true 立即返回；常驻服务同时设置 mode=service。使用 process_read 分页读取日志或有界等待，process_stop 停止。输出日志保留最新4MiB，响应仅提供尾部。不要通过反复轮询等待完成；支持队列的入口会接收完成通知。", Handler: t.SmartExecute},
		{Name: "process_read", Description: "读取当前会话任务状态及合并日志，支持字节游标、输出预算和最长30秒等待终态；轮转缺口以 truncated 标记。", Handler: t.Read},
		{Name: "process_stop", Description: "停止当前会话的命令任务；先发送终止信号，最多等待3秒后强制清理进程组并返回最终状态。", Handler: t.Stop},
		{Name: "process_list", Description: "列出当前会话和工作区所属的命令任务及明确状态。", Handler: t.List},
	})
}

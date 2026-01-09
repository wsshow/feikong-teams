package todo

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取 Todo 工具集合
func GetTools() ([]tool.BaseTool, error) {
	var tools []tool.BaseTool

	// 添加待办事项工具
	todoAddTool, err := utils.InferTool("todo_add", "添加一个新的待办事项。用于记录需要完成的任务或计划。", TodoAdd)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoAddTool)

	// 列出待办事项工具
	todoListTool, err := utils.InferTool("todo_list", "列出所有待办事项，支持按状态和优先级过滤。用于查看当前的任务列表。", TodoListFunc)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoListTool)

	// 更新待办事项工具
	todoUpdateTool, err := utils.InferTool("todo_update", "更新待办事项的信息，包括标题、描述、状态和优先级。用于修改任务信息或更新任务进度。", TodoUpdate)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoUpdateTool)

	// 删除待办事项工具
	todoDeleteTool, err := utils.InferTool("todo_delete", "删除一个待办事项。用于移除已经不需要的任务。", TodoDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoDeleteTool)

	// 批量添加待办事项工具
	todoBatchAddTool, err := utils.InferTool("todo_batch_add", "批量添加多个待办事项。适用于一次性添加多个任务。", TodoBatchAdd)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoBatchAddTool)

	// 批量删除待办事项工具
	todoBatchDeleteTool, err := utils.InferTool("todo_batch_delete", "批量删除多个待办事项。通过提供多个 ID 一次性删除多个任务。", TodoBatchDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoBatchDeleteTool)

	// 清空待办事项工具
	todoClearTool, err := utils.InferTool("todo_clear", "清空待办事项列表。可以选择清空所有待办事项，或仅清空特定状态的待办事项。", TodoClear)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoClearTool)

	return tools, nil
}

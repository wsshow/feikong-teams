package todo

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
)

// GetTools 获取 Todo 工具集合
// tt 是Todo工具实例，包含待办事项存储配置
func (tt *TodoTools) GetTools() ([]runtimeport.Tool, error) {
	if tt == nil {
		return nil, fmt.Errorf("Todo工具未初始化")
	}

	var tools []runtimeport.Tool

	// 添加待办事项工具
	todoAddTool, err := runtimeport.InferTool("todo_add", `添加一个新的待办事项。

仅在复杂、多步骤、跨成员协作，或用户明确要求跟踪进度时使用。不要为单步、简单、纯问答或短小修改创建 TODO。

待办标题应是清晰的动作结果，例如“修复登录失败回归”，描述中写明完成标准。`, tt.TodoAdd)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoAddTool)

	// 列出待办事项工具
	todoListTool, err := runtimeport.InferTool("todo_list", "列出所有待办事项，支持按状态和优先级过滤。用于查看当前任务列表、避免重复创建任务，或在完成一个任务后寻找下一项。", tt.TodoListFunc)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoListTool)

	// 更新待办事项工具
	todoUpdateTool, err := runtimeport.InferTool("todo_update", `更新待办事项的信息，包括标题、描述、状态和优先级。

使用规范：
- 开始执行某项工作前，将对应任务标记为 in_progress。
- 只有完全完成并通过必要验证后，才能标记 completed。
- 遇到错误、阻塞、测试失败、实现不完整时，不要标记 completed；保留 in_progress 或改为 cancelled，并说明原因。
- 需求变化时更新标题/描述，而不是新建重复任务。`, tt.TodoUpdate)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoUpdateTool)

	// 删除待办事项工具
	todoDeleteTool, err := runtimeport.InferTool("todo_delete", "删除一个待办事项。仅用于移除误创建、已废弃或用户要求清理的任务；不要删除仍代表未完成工作的任务。", tt.TodoDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoDeleteTool)

	// 批量添加待办事项工具
	todoBatchAddTool, err := runtimeport.InferTool("todo_batch_add", "批量添加多个待办事项。仅适用于复杂任务拆解后的多个明确步骤；每项都应有清晰目标和完成标准。", tt.TodoBatchAdd)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoBatchAddTool)

	// 批量删除待办事项工具
	todoBatchDeleteTool, err := runtimeport.InferTool("todo_batch_delete", "批量删除多个待办事项。用于任务全部结束后的清理或移除误创建任务；不要删除仍需跟踪的未完成任务。", tt.TodoBatchDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoBatchDeleteTool)

	// 清空待办事项工具
	todoClearTool, err := runtimeport.InferTool("todo_clear", "清空待办事项列表。谨慎使用；只在用户要求、会话任务全部结束，或需要清理 completed/cancelled 项时使用。", tt.TodoClear)
	if err != nil {
		return nil, err
	}
	tools = append(tools, todoClearTool)

	return tools, nil
}

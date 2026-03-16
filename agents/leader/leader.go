package leader

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/file"
	"fkteams/tools/scheduler"
	"fkteams/tools/todo"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

var globalTodoToolsInstance *todo.TodoTools

func NewAgent(ctx context.Context) (adk.Agent, error) {
	safeDir := common.WorkspaceDir()

	todoToolsInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init todo tools: %w", err)
	}
	globalTodoToolsInstance = todoToolsInstance

	todoTools, err := todoToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create todo tools: %w", err)
	}

	fileToolsInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	schedulerInstance, err := scheduler.InitGlobal(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init scheduler: %w", err)
	}
	schedulerTools, err := schedulerInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create scheduler tools: %w", err)
	}

	return common.NewAgentBuilder("统御", "团队管理者，善于规划和分配任务。").
		WithTemplate(LeaderPromptTemplate).
		WithTemplateVar("team_members", ctx.Value("team_members")).
		WithTemplateVar("workspace_dir", safeDir).
		WithTools(todoTools...).
		WithTools(fileTools...).
		WithTools(schedulerTools...).
		WithFullMiddleware(safeDir).
		Build(ctx)
}

func ClearTodoTool() error {
	if globalTodoToolsInstance == nil {
		return nil
	}
	// 使用TodoClear方法清空所有待办事项
	_, err := globalTodoToolsInstance.TodoClear(context.Background(), &todo.TodoClearRequest{})
	if err != nil {
		return err
	}
	return nil
}

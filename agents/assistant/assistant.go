package assistant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	"fkteams/tools/scheduler"
	"fkteams/tools/search"
	"fkteams/tools/todo"
	"fmt"
	"runtime"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()
	safeDir := common.WorkspaceDir()

	smartTools, err := command.NewCommandTools(safeDir).GetTools()
	if err != nil {
		return nil, fmt.Errorf("init command tools: %w", err)
	}

	fileInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	todoInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init todo tools: %w", err)
	}
	todoTools, err := todoInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create todo tools: %w", err)
	}

	schedulerInstance, err := scheduler.InitGlobal(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init scheduler: %w", err)
	}
	schedulerTools, err := schedulerInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create scheduler tools: %w", err)
	}

	searchTool, err := search.NewDuckDuckGoTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("init search tool: %w", err)
	}

	fetchTools, err := fetch.GetTools()
	if err != nil {
		return nil, fmt.Errorf("init fetch tools: %w", err)
	}

	return common.NewAgentBuilder("小助", "个人全能助手，通过命令执行工具和文件操作完成各种任务，危险操作需要用户审批。").
		WithTemplate(AssistantPromptTemplate).
		WithTemplateVar("os_type", runtime.GOOS).
		WithTemplateVar("os_arch", runtime.GOARCH).
		WithTemplateVar("workspace_dir", safeDir).
		WithTools(smartTools...).
		WithTools(fileTools...).
		WithTools(todoTools...).
		WithTools(schedulerTools...).
		WithTools(searchTool).
		WithTools(fetchTools...).
		WithFullMiddleware(safeDir).
		Build(ctx)
}

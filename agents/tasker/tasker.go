package tasker

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	toolSearch "fkteams/tools/search"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	workspaceDir := common.WorkspaceDir()

	duckTool, err := toolSearch.NewDuckDuckGoTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("init search tool: %w", err)
	}

	fetchTools, err := fetch.GetTools()
	if err != nil {
		return nil, fmt.Errorf("init fetch tools: %w", err)
	}

	cmdTools, err := command.NewCommandTools(workspaceDir, command.WithApprovalMode(command.ApprovalModeReject)).GetTools()
	if err != nil {
		return nil, fmt.Errorf("init command tools: %w", err)
	}

	fileToolsInstance, err := file.NewFileTools(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	return common.NewAgentBuilder("任务官", "后台定时任务专属执行官，独立完成信息检索、数据分析、命令执行等各类定时任务，输出严谨可靠的结果。").
		WithTemplate(TaskerPromptTemplate).
		WithTemplateVar("workspace_dir", workspaceDir).
		WithTools(duckTool).
		WithTools(fetchTools...).
		WithTools(cmdTools...).
		WithTools(fileTools...).
		Build(ctx)
}

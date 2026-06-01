package tasker

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	workspaceDir := common.WorkspaceDir()

	cmdTools, err := command.NewCommandTools(workspaceDir, command.WithApprovalMode(command.ApprovalModeReject)).GetTools()
	if err != nil {
		return nil, fmt.Errorf("init command tools: %w", err)
	}

	return common.NewAgentBuilder("tasker", "后台任务执行器，独立完成定时任务中的检索、分析、命令和文件操作。").
		WithTemplate(taskerPromptTemplate).
		WithTemplateVar("workspace_dir", workspaceDir).
		WithTools(cmdTools...).
		WithToolNames("search", "fetch", "file").
		WithSummary().
		Build(ctx)
}

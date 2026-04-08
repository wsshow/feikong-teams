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

	return common.NewAgentBuilder("任务官", "后台定时任务专属执行官，独立完成信息检索、数据分析、命令执行等各类定时任务，输出严谨可靠的结果。").
		WithTemplate(taskerPromptTemplate).
		WithTemplateVar("workspace_dir", workspaceDir).
		WithTools(cmdTools...).
		WithToolNames("search", "fetch", "file").
		WithSummary().
		Build(ctx)
}

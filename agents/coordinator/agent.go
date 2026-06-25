package coordinator

import (
	"context"
	"fkteams/agents/common"
	"fkteams/internal/app/appdata"
	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context, agentTools ...runtimeport.Tool) (runtimeport.Agent, error) {
	safeDir := appdata.WorkspaceDir()

	return common.NewAgentBuilder("coordinator", "核心工程智能体，直接完成常规工程任务，并按需指派专业成员。").
		WithInstruction(coordinatorPrompt).
		WithTemplateVar("workspace_dir", safeDir).
		WithToolNames("todo", "file", "command", "scheduler", "ask").
		WithTools(agentTools...).
		WithSummary().
		WithSkills().
		Build(ctx)
}

package coordinator

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"
	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context, agentTools ...runtimeport.Tool) (runtimeport.Agent, error) {
	return common.BuildAgent(ctx, common.Definition{
		Name:        "coordinator",
		Description: "核心工程智能体，直接完成常规工程任务，并按需指派专业成员。",
		Instruction: coordinatorPrompt,
		Profile:     common.ProfileTeam,
		TemplateVars: map[string]any{
			"workspace_dir": common.WorkspaceDir(),
		},
		ToolNames: []string{"todo", "file", "command", "scheduler", "ask"},
		Tools:     agentTools,
	})
}

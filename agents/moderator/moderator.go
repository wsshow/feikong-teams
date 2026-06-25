package moderator

import (
	"context"
	"fkteams/agents/common"
	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context, agentTools ...runtimeport.Tool) (runtimeport.Agent, error) {
	return common.NewAgentBuilder("moderator", "会议主持人，负责引导讨论、指定发言成员并形成结论。").
		WithInstruction(moderatorPrompt).
		WithTools(agentTools...).
		WithSummary().
		Build(ctx)
}

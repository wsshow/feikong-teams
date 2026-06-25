package coder

import (
	"context"
	"fkteams/agents/common"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	return common.NewAgentBuilder("coder", "软件工程师，负责代码实现、调试、重构和工程验证。").
		WithInstruction(coderPrompt).
		WithToolNames("file", "command").
		WithSummary().
		WithSkills().
		Build(ctx)
}

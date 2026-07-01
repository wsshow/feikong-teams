package coder

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	return common.BuildAgent(ctx, common.Definition{
		Name:        "coder",
		Description: "软件工程师，负责代码实现、调试、重构和工程验证。",
		Instruction: coderPrompt,
		Profile:     common.ProfileFull,
		ToolNames:   []string{"file", "command"},
	})
}

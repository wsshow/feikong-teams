package moderator

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"
	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context, agentTools ...runtimeport.Tool) (runtimeport.Agent, error) {
	def := DefaultDefinition()
	def.Tools = append(def.Tools, agentTools...)
	return common.BuildAgent(ctx, def)
}

// DefaultDefinition 返回默认智能体声明。
func DefaultDefinition() common.Definition {
	return common.Definition{
		Name:          "moderator",
		Description:   "会议主持人，负责引导讨论、指定发言成员并形成结论。",
		Instruction:   moderatorPrompt,
		Profile:       common.ProfileWorkspace,
		EnableSummary: true,
	}
}

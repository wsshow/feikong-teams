package researcher

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	return common.BuildAgent(ctx, common.Definition{
		Name:          "researcher",
		Description:   "网络研究员，负责检索、抓取、交叉验证和整理时效信息。",
		Instruction:   researcherPrompt,
		Profile:       common.ProfileWorkspace,
		ToolNames:     []string{"search", "fetch"},
		EnableSummary: true,
	})
}

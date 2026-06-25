package analyst

import (
	"context"
	"fkteams/agents/common"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	return common.NewAgentBuilder("analyst", "数据分析师，负责使用表格、脚本和文档工具提取洞察。").
		WithInstruction(analystPrompt).
		WithToolNames("todo", "excel", "file", "uv", "doc").
		WithSummary().
		WithSkills().
		Build(ctx)
}

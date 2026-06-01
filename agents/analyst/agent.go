package analyst

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("analyst", "数据分析师，负责使用表格、脚本和文档工具提取洞察。").
		WithTemplate(analystPromptTemplate).
		WithToolNames("todo", "excel", "file", "uv", "doc").
		WithSummary().
		WithSkills().
		Build(ctx)
}

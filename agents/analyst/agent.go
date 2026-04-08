package analyst

import (
	"context"
	"fkteams/agents/common"
	"runtime"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小析", "数据分析专家，擅长使用 Excel、Python 脚本和文档处理工具，从复杂数据和文档中提取有价值的信息并提供专业洞察。").
		WithTemplate(analystPromptTemplate).
		WithTemplateVar("os", runtime.GOOS).
		WithTemplateVar("workspace_dir", common.WorkspaceDir()).
		WithToolNames("todo", "excel", "file", "uv", "doc").
		WithSummary().
		WithSkills().
		Build(ctx)
}

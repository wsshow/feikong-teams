package assistant

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("generalist", "通用执行助手，负责综合命令、文件、搜索和文档工具完成开放任务。").
		WithTemplate(assistantPromptTemplate).
		WithToolNames("command", "file", "search", "fetch", "ask", "doc").
		WithDispatch(nil).
		WithSummary().
		WithSkills().
		Build(ctx)
}

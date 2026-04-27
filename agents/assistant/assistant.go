package assistant

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小助", "个人全能助手，通过命令执行、文件操作和网络搜索完成各种任务，支持将多个独立子任务并行分发执行。").
		WithTemplate(assistantPromptTemplate).
		WithToolNames("command", "file", "search", "fetch", "ask", "doc").
		WithDispatch(nil).
		WithSummary().
		WithSkills().
		Build(ctx)
}

package cmder

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小令", "命令行专家，擅长通过命令行操作完成任务，能够根据操作系统环境执行合适的命令。").
		WithTemplate(cmderPromptTemplate).
		WithToolNames("command").
		WithSummary().
		WithSkills().
		Build(ctx)
}

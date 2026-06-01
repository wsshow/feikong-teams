package cmder

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("shell", "命令行专家，负责使用终端命令完成系统、文件和脚本操作。").
		WithTemplate(cmderPromptTemplate).
		WithToolNames("command").
		WithSummary().
		WithSkills().
		Build(ctx)
}

package assistant

import (
	"context"
	"fkteams/agents/common"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	return common.NewAgentBuilder("generalist", "通用执行助手，负责综合命令、文件、搜索和文档工具完成开放任务。").
		WithInstruction(assistantPrompt).
		WithToolNames("command", "file", "search", "fetch", "ask", "doc").
		WithDispatch(nil).
		WithSummary().
		WithSkills().
		Build(ctx)
}

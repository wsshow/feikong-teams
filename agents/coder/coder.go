package coder

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("coder", "软件工程师，负责代码实现、调试、重构和工程验证。").
		WithTemplate(coderPromptTemplate).
		WithToolNames("file", "command").
		WithSummary().
		WithSkills().
		Build(ctx)
}

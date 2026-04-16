package coder

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小码", "资深软件工程师，擅长代码实现、调试和重构，遵循项目约定并验证变更正确性。").
		WithTemplate(coderPromptTemplate).
		WithToolNames("file", "command").
		WithSummary().
		WithSkills().
		Build(ctx)
}

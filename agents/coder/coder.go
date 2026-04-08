package coder

import (
	"context"
	"fkteams/agents/common"
	"runtime"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小码", "资深软件工程师，擅长代码实现、调试和重构，遵循项目约定并验证变更正确性。").
		WithTemplate(coderPromptTemplate).
		WithTemplateVar("code_dir", common.WorkspaceDir()).
		WithTemplateVar("os_type", runtime.GOOS).
		WithToolNames("file", "command").
		WithSummary().
		WithSkills().
		Build(ctx)
}

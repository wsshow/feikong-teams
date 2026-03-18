package assistant

import (
	"context"
	"fkteams/agents/common"
	"runtime"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	safeDir := common.WorkspaceDir()

	return common.NewAgentBuilder("小助", "个人全能助手，通过命令执行、文件操作和网络搜索完成各种任务，支持将多个独立子任务并行分发执行。").
		WithTemplate(assistantPromptTemplate).
		WithTemplateVar("os_type", runtime.GOOS).
		WithTemplateVar("os_arch", runtime.GOARCH).
		WithTemplateVar("workspace_dir", safeDir).
		WithToolNames("command", "file", "search", "fetch").
		WithDispatch(nil).
		WithSummary().
		WithSkills(safeDir).
		Build(ctx)
}

package assistant

import (
	"context"
	"fkteams/agents/common"
	"runtime"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	safeDir := common.WorkspaceDir()

	return common.NewAgentBuilder("小助", "个人全能助手，通过命令执行工具和文件操作完成各种任务，危险操作需要用户审批。").
		WithTemplate(assistantPromptTemplate).
		WithTemplateVar("os_type", runtime.GOOS).
		WithTemplateVar("os_arch", runtime.GOARCH).
		WithTemplateVar("workspace_dir", safeDir).
		WithToolNames("command", "file", "todo", "scheduler", "search", "fetch").
		WithSummary().
		WithSkills(safeDir).
		Build(ctx)
}

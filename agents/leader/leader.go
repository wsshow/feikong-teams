package leader

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	safeDir := common.WorkspaceDir()

	return common.NewAgentBuilder("统御", "团队管理者，善于规划和分配任务。").
		WithTemplate(leaderPromptTemplate).
		WithTemplateVar("workspace_dir", safeDir).
		WithToolNames("todo", "file", "scheduler", "ask").
		WithSummary().
		WithSkills().
		Build(ctx)
}

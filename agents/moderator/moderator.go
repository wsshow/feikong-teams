package moderator

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

func NewAgent(ctx context.Context, agentTools ...tool.BaseTool) (adk.Agent, error) {
	return common.NewAgentBuilder("moderator", "会议主持人，负责引导讨论、指定发言成员并形成结论。").
		WithTemplate(moderatorPromptTemplate).
		WithTools(agentTools...).
		WithSummary().
		Build(ctx)
}

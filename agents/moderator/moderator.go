package moderator

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

func NewAgent(ctx context.Context, agentTools ...tool.BaseTool) (adk.Agent, error) {
	return common.NewAgentBuilder("小议", "会议主持人，负责引导讨论并确保各成员积极参与。").
		WithTemplate(moderatorPromptTemplate).
		WithTools(agentTools...).
		WithSummary().
		Build(ctx)
}

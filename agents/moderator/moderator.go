package moderator

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	return common.NewAgentBuilder("小议", "会议主持人，负责引导讨论并确保各成员积极参与。").
		WithTemplate(moderatorPromptTemplate).
		Build(context.Background())
}

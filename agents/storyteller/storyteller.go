package storyteller

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("writer", "创意写作专家，负责故事、文案、叙事和表达润色。").
		WithTemplate(storytellerPromptTemplate).
		WithSummary().
		WithSkills().
		Build(ctx)
}

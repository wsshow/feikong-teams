package storyteller

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小说", "故事创作专家，擅长构思和撰写引人入胜的故事、小说和创意文本。").
		WithTemplate(storytellerPromptTemplate).
		WithSummary().
		WithSkills().
		Build(ctx)
}

package storyteller

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	return common.NewAgentBuilder("小天", "讲故事专家，擅长编写引人入胜的故事。").
		WithTemplate(StorytellerPromptTemplate).
		Build(context.Background())
}

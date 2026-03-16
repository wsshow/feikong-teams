package summarizer

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小简", "总结专家，擅长提炼和总结复杂信息，将冗长内容提炼为精炼的要点摘要。").
		WithTemplate(summarizerPromptTemplate).
		Build(ctx)
}

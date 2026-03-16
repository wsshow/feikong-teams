package summarizer

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	return common.NewAgentBuilder("小简", "总结专家，擅长将冗长的信息提炼为简洁的摘要。").
		WithTemplate(summarizerPromptTemplate).
		Build(context.Background())
}

package summarizer

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("summarizer", "总结专家，负责提炼冗长信息、压缩上下文和输出清晰摘要。").
		WithTemplate(summarizerPromptTemplate).
		Build(ctx)
}

package searcher

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("researcher", "网络研究员，负责检索、抓取、交叉验证和整理时效信息。").
		WithTemplate(searcherPromptTemplate).
		WithToolNames("search", "fetch").
		WithSummary().
		Build(ctx)
}

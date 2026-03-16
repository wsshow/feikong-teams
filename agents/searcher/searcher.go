package searcher

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	return common.NewAgentBuilder("小搜", "搜索专家，擅长通过DuckDuckGo搜索引擎检索信息，并利用Fetch工具抓取网页内容进行深度分析，提供准确、实时的情报服务。").
		WithTemplate(searcherPromptTemplate).
		WithToolNames("search", "fetch").
		Build(ctx)
}

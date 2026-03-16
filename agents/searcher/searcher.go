package searcher

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/fetch"
	toolSearch "fkteams/tools/search"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()
	duckTool, err := toolSearch.NewDuckDuckGoTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("init search tool: %w", err)
	}
	fetchTools, err := fetch.GetTools()
	if err != nil {
		return nil, fmt.Errorf("init fetch tools: %w", err)
	}

	return common.NewAgentBuilder("小搜", "搜索专家，擅长通过DuckDuckGo搜索引擎检索信息，并利用Fetch工具抓取网页内容进行深度分析，提供准确、实时的情报服务。").
		WithTemplate(SearcherPromptTemplate).
		WithTools(duckTool).
		WithTools(fetchTools...).
		Build(ctx)
}

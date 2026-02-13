package searcher

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/fetch"
	toolSearch "fkteams/tools/search"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()
	duckTool, err := toolSearch.NewDuckDuckGoTool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fetchTool, err := fetch.GetTools()
	if err != nil {
		log.Fatal(err)
	}
	var toolList []tool.BaseTool
	toolList = append(toolList, duckTool)
	toolList = append(toolList, fetchTool...)

	systemMessages, err := SearcherPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小搜",
		Description:   "搜索专家，擅长通过DuckDuckGo搜索引擎检索信息，并利用Fetch工具抓取网页内容进行深度分析，提供准确、实时的情报服务。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

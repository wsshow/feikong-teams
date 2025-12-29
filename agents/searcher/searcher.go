package searcher

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()
	duckTool, err := tools.NewDuckDuckGoTool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	systemMessages, err := SearcherPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "小搜",
		Description: "搜索专家，擅长通过DuckDuckGo提供准确的信息搜索服务。",
		Instruction: instruction,
		Model:       common.NewChatModel(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{duckTool},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

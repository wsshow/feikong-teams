package coder

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()

	// 创建文件操作工具
	fileTools, err := tools.GetFileTools()
	if err != nil {
		log.Fatal(err)
	}

	// 格式化系统消息
	systemMessages, err := CoderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	// 创建智能体
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "小码",
		Description: "代码专家，擅长读写和处理代码文件，能够帮助用户完成各种编程任务。",
		Instruction: instruction,
		Model:       common.NewChatModel(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: fileTools,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

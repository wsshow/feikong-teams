package moderator

import (
	"context"
	"fkteams/agents/common"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() adk.Agent {
	ctx := context.Background()
	systemMessages, err := ModeratorPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小意",
		Description:   "意图识别专家，擅长分析用户请求并判断其背后的真实意图。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

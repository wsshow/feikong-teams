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
		Name:          "小议",
		Description:   "会议主持人，负责引导讨论并确保各成员积极参与。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

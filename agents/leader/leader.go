package leader

import (
	"context"
	"fkteams/agents/common"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() adk.Agent {
	ctx := context.Background()
	systemMessages, err := LeaderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "统御",
		Description: "团队管理者，善于规划和分配任务。",
		Instruction: instruction,
		Model:       common.NewChatModel(),
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

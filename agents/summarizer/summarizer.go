package summarizer

import (
	"context"
	"fkteams/agents/common"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() adk.Agent {
	ctx := context.Background()
	systemMessages, err := SummarizerPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小简",
		Description:   "总结专家，擅长将冗长的信息提炼为简洁的摘要。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

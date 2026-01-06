package discussant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(member config.TeamMember) adk.Agent {
	ctx := context.Background()
	systemMessages, err := DiscussantPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          member.Name,
		Description:   member.Desc,
		Instruction:   instruction,
		Model:         common.NewChatModelWithConfig(member.ModelName, member.BaseURL, member.APIKey),
		MaxIterations: common.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

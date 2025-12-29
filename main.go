package main

import (
	"context"
	"fkteams/agents/leader"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fmt"
	"io"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	leaderAgent := leader.NewAgent()

	ctx := context.Background()
	a, err := adk.SetSubAgents(ctx, leaderAgent, []adk.Agent{searcherAgent, storytellerAgent})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           a,
		EnableStreaming: true,
	})

	for {
		input, _ := pterm.DefaultInteractiveTextInput.Show("请输入您的问题")
		if input == "q" || input == "quit" || input == "" {
			pterm.Println("谢谢使用，再见！")
			break
		}
		iter := runner.Query(ctx, input)
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				log.Println("error:", event.Err)
				continue
			}

			if event.Output.MessageOutput.Role == schema.Tool {
				fmt.Printf("\n\ntool name: %s \ntool tool_calls: %+v\ntool content: %s\n\n",
					event.Output.MessageOutput.ToolName,
					event.Output.MessageOutput.Message,
					event.Output.MessageOutput.Message.Content)
				continue
			}

			if event.Output.MessageOutput.MessageStream == nil {
				fmt.Printf("\n%s\n\n", event.Output.MessageOutput.Message)
				continue
			}

			for {
				msg, err := event.Output.MessageOutput.MessageStream.Recv()
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Fatal(err)
				}
				fmt.Print(msg.Content)
			}
		}
	}
}

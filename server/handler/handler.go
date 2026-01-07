package handler

import (
	"context"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/discussant"
	"fkteams/agents/leader"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/visitor"
	"fkteams/common"
	"fkteams/config"
	"fkteams/fkevent"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func RoundtableHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var param struct {
			SessionId string `json:"session_id" binding:"required"`
			Message   string `json:"message" binding:"required"`
		}
		if err := c.ShouldBindJSON(&param); err != nil {
			log.Println(err)
			c.JSON(http.StatusOK, resp.Failure().WithDesc("参数错误"))
			return
		}
		var inputMessages []adk.Message
		input := param.Message
		inputMessages = []adk.Message{}
		historyFilePath := fmt.Sprintf("./history/chat_history/fkteams_chat_history_%s", param.SessionId)
		err := fkevent.GlobalHistoryRecorder.LoadFromFile(historyFilePath)
		if err == nil {
			log.Printf("自动加载聊天历史: [%s]", param.SessionId)
		}
		agentMessages := fkevent.GlobalHistoryRecorder.GetMessages()
		if len(agentMessages) > 0 {
			var historyMessage strings.Builder
			for _, agentMessage := range agentMessages {
				fmt.Fprintf(&historyMessage, "%s: %s\n", agentMessage.AgentName, agentMessage.Content)
			}
			inputMessages = append(inputMessages, schema.SystemMessage(fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String())))
		}
		inputMessages = append(inputMessages, schema.UserMessage(input))
		fkevent.GlobalHistoryRecorder.RecordUserInput(input)

		ctx := context.Background()
		runner := loopAgentMode(ctx)
		fkevent.Callback = func(event fkevent.Event) error {
			fmt.Println(event)
			return nil
		}
		iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
				log.Printf("Error processing event: %v", err)
				break
			}
		}
		log.Printf("任务完成，正在自动保存聊天历史到 %s ...", historyFilePath)
		err = fkevent.GlobalHistoryRecorder.SaveToFile(historyFilePath)
		if err != nil {
			log.Printf("保存聊天历史失败: %v", err)
		} else {
			log.Printf("成功保存聊天历史到文件: %s", historyFilePath)
		}
	}
}

func SupervisorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var param struct {
			SessionId string `json:"session_id" binding:"required"`
			Message   string `json:"message" binding:"required"`
		}
		if err := c.ShouldBindJSON(&param); err != nil {
			log.Println(err)
			c.JSON(http.StatusOK, resp.Failure().WithDesc("参数错误"))
			return
		}
		var inputMessages []adk.Message
		input := param.Message
		inputMessages = []adk.Message{}
		historyFilePath := fmt.Sprintf("./history/chat_history/fkteams_chat_history_%s", param.SessionId)
		err := fkevent.GlobalHistoryRecorder.LoadFromFile(historyFilePath)
		if err == nil {
			log.Printf("自动加载聊天历史: [%s]", param.SessionId)
		}
		agentMessages := fkevent.GlobalHistoryRecorder.GetMessages()
		if len(agentMessages) > 0 {
			var historyMessage strings.Builder
			for _, agentMessage := range agentMessages {
				fmt.Fprintf(&historyMessage, "%s: %s\n", agentMessage.AgentName, agentMessage.Content)
			}
			inputMessages = append(inputMessages, schema.SystemMessage(fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String())))
		}
		inputMessages = append(inputMessages, schema.UserMessage(input))
		fkevent.GlobalHistoryRecorder.RecordUserInput(input)

		ctx := context.Background()
		runner := supervisorMode(ctx)
		fkevent.Callback = func(event fkevent.Event) error {
			fmt.Println(event)
			return nil
		}
		iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
				log.Printf("Error processing event: %v", err)
				break
			}
		}
		log.Printf("任务完成，正在自动保存聊天历史到 %s ...", historyFilePath)
		err = fkevent.GlobalHistoryRecorder.SaveToFile(historyFilePath)
		if err != nil {
			log.Printf("保存聊天历史失败: %v", err)
		} else {
			log.Printf("成功保存聊天历史到文件: %s", historyFilePath)
		}
	}
}

func supervisorMode(ctx context.Context) *adk.Runner {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	leaderAgent := leader.NewAgent()
	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	subAgents := []adk.Agent{searcherAgent, storytellerAgent}

	if os.Getenv("FEIKONG_CODER_ENABLED") == "true" {
		coderAgent := coder.NewAgent()
		subAgents = append(subAgents, coderAgent)
	}

	if os.Getenv("FEIKONG_CMDER_ENABLED") == "true" {
		cmderAgent := cmder.NewAgent()
		subAgents = append(subAgents, cmderAgent)
	}

	if os.Getenv("FEIKONG_SSH_VISITOR_ENABLED") == "true" {
		visitorAgent := visitor.NewAgent()
		defer visitor.CloseSSHClient()
		subAgents = append(subAgents, visitorAgent)
	}

	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: leaderAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           supervisorAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}

func loopAgentMode(ctx context.Context) *adk.Runner {
	teamConfig, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent := discussant.NewAgent(member)
		subAgents = append(subAgents, agent)
	}

	loopAgent, err := adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          "Roundtable",
		Description:   "多智能体共同讨论并解决问题",
		SubAgents:     subAgents,
		MaxIterations: teamConfig.Roundtable.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           loopAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}

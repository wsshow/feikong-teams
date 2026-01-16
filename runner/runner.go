package runner

import (
	"context"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/custom"
	"fkteams/agents/discussant"
	"fkteams/agents/leader"
	"fkteams/agents/moderator"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/visitor"
	"fkteams/common"
	"fkteams/config"
	"fkteams/g"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
)

// CreateSupervisorRunner 创建 Supervisor 模式的 Runner
func CreateSupervisorRunner(ctx context.Context) *adk.Runner {
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
		g.Cleaner.Add(func() error {
			visitor.CloseSSHClient()
			return nil
		})
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

// CreateLoopAgentRunner 创建 LoopAgent 模式的 Runner
func CreateLoopAgentRunner(ctx context.Context) *adk.Runner {
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

// CreateCustomSupervisorRunner 创建自定义 Supervisor 模式的 Runner
func CreateCustomSupervisorRunner(ctx context.Context) *adk.Runner {
	cfg, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent

	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent = custom.NewAgent(custom.Config{
			Name:         cfg.Custom.Moderator.Name,
			Description:  cfg.Custom.Moderator.Desc,
			SystemPrompt: cfg.Custom.Moderator.SystemPrompt,
			Model: custom.Model{
				Name:    cfg.Custom.Moderator.ModelName,
				APIKey:  cfg.Custom.Moderator.APIKey,
				BaseURL: cfg.Custom.Moderator.BaseURL,
			},
			ToolNames: cfg.Custom.Moderator.Tools,
		})
	} else {
		moderatorAgent = moderator.NewAgent()
	}

	for _, customAgent := range cfg.Custom.Agents {
		subAgents = append(subAgents, custom.NewAgent(custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model: custom.Model{
				Name:    customAgent.ModelName,
				APIKey:  customAgent.APIKey,
				BaseURL: customAgent.BaseURL,
			},
			ToolNames: customAgent.Tools,
		}))
	}

	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: moderatorAgent,
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

// PrintCustomAgentsInfo 打印自定义模式的智能体信息
func PrintCustomAgentsInfo(ctx context.Context) {
	cfg, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent

	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent = custom.NewAgent(custom.Config{
			Name:         cfg.Custom.Moderator.Name,
			Description:  cfg.Custom.Moderator.Desc,
			SystemPrompt: cfg.Custom.Moderator.SystemPrompt,
			Model: custom.Model{
				Name:    cfg.Custom.Moderator.ModelName,
				APIKey:  cfg.Custom.Moderator.APIKey,
				BaseURL: cfg.Custom.Moderator.BaseURL,
			},
			ToolNames: cfg.Custom.Moderator.Tools,
		})
	} else {
		moderatorAgent = moderator.NewAgent()
	}

	for _, customAgent := range cfg.Custom.Agents {
		subAgents = append(subAgents, custom.NewAgent(custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model: custom.Model{
				Name:    customAgent.ModelName,
				APIKey:  customAgent.APIKey,
				BaseURL: customAgent.BaseURL,
			},
			ToolNames: customAgent.Tools,
		}))
	}

	fmt.Printf("本次讨论的主持人: %s\n", moderatorAgent.Name(ctx))
	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))
}

// PrintLoopAgentsInfo 打印多智能体讨论模式的智能体信息
func PrintLoopAgentsInfo(ctx context.Context) {
	teamConfig, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent := discussant.NewAgent(member)
		subAgents = append(subAgents, agent)
	}

	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))
}

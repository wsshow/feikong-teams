// Package runner 提供各工作模式的 Runner 创建工厂函数
package runner

import (
	"context"
	"fkteams/agents"
	"fkteams/agents/custom"
	"fkteams/agents/deep"
	"fkteams/agents/discussant"
	"fkteams/agents/leader"
	"fkteams/agents/moderator"
	"fkteams/agents/tasker"
	"fkteams/common"
	"fkteams/config"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
)

// newRunner 用共享配置创建 Runner
func newRunner(ctx context.Context, agent adk.Agent) *adk.Runner {
	return adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})
}

// CreateBackgroundTaskRunner 创建后台定时任务专用 Runner（任务官单智能体，独立执行）
func CreateBackgroundTaskRunner(ctx context.Context) (*adk.Runner, error) {
	agent, err := tasker.NewAgent(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建任务官智能体失败: %w", err)
	}
	return newRunner(ctx, agent), nil
}

// CreateAgentRunner 创建普通 ReACT 模式的 Runner
func CreateAgentRunner(ctx context.Context, agent adk.Agent) *adk.Runner {
	return newRunner(ctx, agent)
}

// CreateSupervisorRunner 创建 Supervisor 模式的 Runner
func CreateSupervisorRunner(ctx context.Context) (*adk.Runner, error) {
	subAgents := agents.GetTeamAgents(ctx)

	organizeCtx := context.WithValue(ctx, "team_members", strings.Join(func() []string {
		var names []string
		for _, agent := range subAgents {
			names = append(names, agent.Name(ctx))
		}
		return names
	}(), ", "))

	leaderAgent, err := leader.NewAgent(organizeCtx)
	if err != nil {
		return nil, fmt.Errorf("创建统御智能体失败: %w", err)
	}
	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: leaderAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Supervisor 失败: %w", err)
	}

	return newRunner(ctx, supervisorAgent), nil
}

// CreateDeepAgentsRunner 创建 DeepAgents 模式的 Runner
func CreateDeepAgentsRunner(ctx context.Context) (*adk.Runner, error) {
	subAgents := agents.GetTeamAgents(ctx)

	supervisorAgent, err := deep.NewAgent(ctx, subAgents)
	if err != nil {
		return nil, fmt.Errorf("创建 DeepAgents 失败: %w", err)
	}

	return newRunner(ctx, supervisorAgent), nil
}

// CreateLoopAgentRunner 创建 LoopAgent 模式的 Runner
func CreateLoopAgentRunner(ctx context.Context) (*adk.Runner, error) {
	teamConfig, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("获取配置失败: %w", err)
	}

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent, err := discussant.NewAgent(member)
		if err != nil {
			return nil, fmt.Errorf("创建讨论智能体 %s 失败: %w", member.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	loopAgent, err := adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          "Roundtable",
		Description:   "多智能体共同讨论并解决问题",
		SubAgents:     subAgents,
		MaxIterations: teamConfig.Roundtable.MaxIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 LoopAgent 失败: %w", err)
	}

	return newRunner(ctx, loopAgent), nil
}

// CreateCustomSupervisorRunner 创建自定义 Supervisor 模式的 Runner
func CreateCustomSupervisorRunner(ctx context.Context) (*adk.Runner, error) {
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("获取配置失败: %w", err)
	}

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent

	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent, err = custom.NewAgent(custom.Config{
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
		if err != nil {
			return nil, fmt.Errorf("创建自定义主持人失败: %w", err)
		}
	} else {
		moderatorAgent, err = moderator.NewAgent()
		if err != nil {
			return nil, fmt.Errorf("创建主持人失败: %w", err)
		}
	}

	for _, customAgent := range cfg.Custom.Agents {
		agent, err := custom.NewAgent(custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model: custom.Model{
				Name:    customAgent.ModelName,
				APIKey:  customAgent.APIKey,
				BaseURL: customAgent.BaseURL,
			},
			ToolNames: customAgent.Tools,
		})
		if err != nil {
			return nil, fmt.Errorf("创建自定义智能体 %s 失败: %w", customAgent.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: moderatorAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		return nil, fmt.Errorf("创建自定义 Supervisor 失败: %w", err)
	}

	return newRunner(ctx, supervisorAgent), nil
}

// PrintCustomAgentsInfo 打印自定义模式的智能体信息
func PrintCustomAgentsInfo(ctx context.Context) error {
	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("获取配置失败: %w", err)
	}

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent

	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent, err = custom.NewAgent(custom.Config{
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
		if err != nil {
			return fmt.Errorf("创建自定义主持人失败: %w", err)
		}
	} else {
		moderatorAgent, err = moderator.NewAgent()
		if err != nil {
			return fmt.Errorf("创建主持人失败: %w", err)
		}
	}

	for _, customAgent := range cfg.Custom.Agents {
		agent, err := custom.NewAgent(custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model: custom.Model{
				Name:    customAgent.ModelName,
				APIKey:  customAgent.APIKey,
				BaseURL: customAgent.BaseURL,
			},
			ToolNames: customAgent.Tools,
		})
		if err != nil {
			return fmt.Errorf("创建自定义智能体 %s 失败: %w", customAgent.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	fmt.Printf("本次讨论的主持人: %s\n", moderatorAgent.Name(ctx))
	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))
	return nil
}

// PrintLoopAgentsInfo 打印多智能体讨论模式的智能体信息
func PrintLoopAgentsInfo(ctx context.Context) error {
	teamConfig, err := config.Get()
	if err != nil {
		return fmt.Errorf("获取配置失败: %w", err)
	}

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent, err := discussant.NewAgent(member)
		if err != nil {
			return fmt.Errorf("创建讨论智能体 %s 失败: %w", member.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))
	return nil
}

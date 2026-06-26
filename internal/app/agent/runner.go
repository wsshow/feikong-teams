// Package agent 提供智能体解析、团队组装和 Runner 创建用例。
package agent

import (
	"context"
	"fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/coordinator"
	"fkteams/internal/app/agent/catalog/custom"
	"fkteams/internal/app/agent/catalog/deep"
	"fkteams/internal/app/agent/catalog/discussant"
	"fkteams/internal/app/agent/catalog/moderator"
	"fkteams/internal/app/agent/catalog/tasker"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/config"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/checkpoint"
	"fmt"
	"regexp"
	"strings"
)

var validToolNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func agentToolName(name string, index int, used map[string]bool) string {
	normalized := strings.ToLower(validToolNameChars.ReplaceAllString(name, "_"))
	normalized = strings.Trim(normalized, "_-")
	if normalized == "" || (normalized[0] >= '0' && normalized[0] <= '9') {
		normalized = fmt.Sprintf("member_%d", index+1)
	}
	normalized = toolmeta.AgentToolPrefix + normalized

	base := normalized
	for suffix := 2; used[normalized]; suffix++ {
		normalized = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[normalized] = true
	return normalized
}

func buildAgentTools(ctx context.Context, subAgents []runtimeport.Agent) ([]runtimeport.Tool, error) {
	engine, err := runtimeport.RequireEngine(ctx)
	if err != nil {
		return nil, err
	}
	usedNames := make(map[string]bool, len(subAgents))
	return engine.NewAgentTools(ctx, subAgents, runtimeport.AgentToolConfig{
		ToolName: func(displayName string, index int) string {
			return agentToolName(displayName, index, usedNames)
		},
		RegisterDisplay: toolmeta.RegisterAgentToolDisplay,
	})
}

// resolveCustomModel 从配置文件解析自定义智能体的模型配置
func resolveCustomModel(cfg *config.Config, agent config.CustomAgent) custom.Model {
	mc := cfg.ResolveModel(agent.Model)
	if mc == nil {
		return custom.Model{}
	}
	return custom.Model{
		Provider: mc.Provider,
		Name:     mc.Model,
		APIKey:   mc.APIKey,
		BaseURL:  mc.BaseURL,
	}
}

// newRunner 用共享配置创建 Runner
func newRunner(ctx context.Context, agent runtimeport.Agent) (runtimeport.Runner, error) {
	engine, err := runtimeport.RequireEngine(ctx)
	if err != nil {
		return nil, err
	}
	return engine.NewRunner(ctx, runtimeport.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckpointStore: checkpoint.NewMemoryStore(),
	})
}

// CreateBackgroundTaskRunner 创建后台定时任务专用 Runner（任务官单智能体，独立执行）
func CreateBackgroundTaskRunner(ctx context.Context) (runtimeport.Runner, error) {
	agent, err := tasker.NewAgent(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建任务官智能体失败: %w", err)
	}
	return newRunner(ctx, agent)
}

// CreateAgentRunner 创建普通 ReACT 模式的 Runner
func CreateAgentRunner(ctx context.Context, agent runtimeport.Agent) (runtimeport.Runner, error) {
	return newRunner(ctx, agent)
}

// CreateTeamRunner 创建团队模式 Runner，使用 ChatModelAgent + AgentTool 协作。
func CreateTeamRunner(ctx context.Context) (runtimeport.Runner, error) {
	subAgents, err := agents.TeamAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建团队智能体失败: %w", err)
	}
	agentTools, err := buildAgentTools(ctx, subAgents)
	if err != nil {
		return nil, fmt.Errorf("创建成员工具失败: %w", err)
	}

	coordinatorAgent, err := coordinator.NewAgent(ctx, agentTools...)
	if err != nil {
		return nil, fmt.Errorf("创建 coordinator 智能体失败: %w", err)
	}

	return newRunner(ctx, coordinatorAgent)
}

// CreateDeepAgentsRunner 创建 DeepAgents 模式的 Runner
func CreateDeepAgentsRunner(ctx context.Context) (runtimeport.Runner, error) {
	subAgents, err := agents.TeamAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建团队智能体失败: %w", err)
	}

	deepAgent, err := deep.NewAgent(ctx, subAgents)
	if err != nil {
		return nil, fmt.Errorf("创建 DeepAgents 失败: %w", err)
	}

	return newRunner(ctx, deepAgent)
}

// CreateLoopAgentRunner 创建 LoopAgent 模式的 Runner
func CreateLoopAgentRunner(ctx context.Context) (runtimeport.Runner, error) {
	teamConfig := config.Get()

	var subAgents []runtimeport.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent, err := discussant.NewAgent(ctx, member)
		if err != nil {
			return nil, fmt.Errorf("创建讨论智能体 %s 失败: %w", member.Name, err)
		}
		subAgents = append(subAgents, agent)
	}
	engine, err := runtimeport.RequireEngine(ctx)
	if err != nil {
		return nil, err
	}
	loopAgent, err := engine.NewLoopAgent(ctx, &runtimeport.LoopAgentConfig{
		Name:          "Roundtable",
		Description:   "多智能体共同讨论并解决问题",
		SubAgents:     subAgents,
		MaxIterations: teamConfig.Roundtable.MaxIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 LoopAgent 失败: %w", err)
	}

	return newRunner(ctx, loopAgent)
}

// CreateCustomRunner 创建自定义会议模式 Runner，使用主持人 ChatModelAgent + AgentTool 协作。
func CreateCustomRunner(ctx context.Context) (runtimeport.Runner, error) {
	cfg := config.Get()

	var moderatorAgent runtimeport.Agent
	var subAgents []runtimeport.Agent
	var err error

	for _, customAgent := range cfg.Custom.Agents {
		agent, err := custom.NewAgent(ctx, custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model:        resolveCustomModel(cfg, customAgent),
			ToolNames:    customAgent.Tools,
		})
		if err != nil {
			return nil, fmt.Errorf("创建自定义智能体 %s 失败: %w", customAgent.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	agentTools, err := buildAgentTools(ctx, subAgents)
	if err != nil {
		return nil, fmt.Errorf("创建成员工具失败: %w", err)
	}
	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent, err = custom.NewAgent(ctx, custom.Config{
			Name:         cfg.Custom.Moderator.Name,
			Description:  cfg.Custom.Moderator.Desc,
			SystemPrompt: customModeratorPrompt(cfg.Custom.Moderator.SystemPrompt),
			Model:        resolveCustomModel(cfg, cfg.Custom.Moderator),
			ToolNames:    cfg.Custom.Moderator.Tools,
			Tools:        agentTools,
		})
		if err != nil {
			return nil, fmt.Errorf("创建自定义主持人失败: %w", err)
		}
	} else {
		moderatorAgent, err = moderator.NewAgent(ctx, agentTools...)
		if err != nil {
			return nil, fmt.Errorf("创建主持人失败: %w", err)
		}
	}

	return newRunner(ctx, moderatorAgent)
}

func customModeratorPrompt(systemPrompt string) string {
	if systemPrompt == "" {
		systemPrompt = "你是一个公正的主持人，负责根据任务需求协调成员协作。"
	}
	return systemPrompt + `

---

## 子智能体工具
可用的成员已经作为工具提供。需要成员执行任务、补充观点或发言时，调用对应工具，并在 request 中写明目标、上下文和期望输出。
成员返回后，由你负责整理、追问下一位成员或形成最终结论。`
}

// PrintCustomAgentsInfo 打印自定义模式的智能体信息
func PrintCustomAgentsInfo(ctx context.Context) error {
	cfg := config.Get()

	var moderatorAgent runtimeport.Agent
	var subAgents []runtimeport.Agent
	var err error

	if cfg.Custom.Moderator.Name != "" {
		moderatorAgent, err = custom.NewAgent(ctx, custom.Config{
			Name:         cfg.Custom.Moderator.Name,
			Description:  cfg.Custom.Moderator.Desc,
			SystemPrompt: cfg.Custom.Moderator.SystemPrompt,
			Model:        resolveCustomModel(cfg, cfg.Custom.Moderator),
			ToolNames:    cfg.Custom.Moderator.Tools,
		})
		if err != nil {
			return fmt.Errorf("创建自定义主持人失败: %w", err)
		}
	} else {
		moderatorAgent, err = moderator.NewAgent(ctx)
		if err != nil {
			return fmt.Errorf("创建主持人失败: %w", err)
		}
	}

	for _, customAgent := range cfg.Custom.Agents {
		agent, err := custom.NewAgent(ctx, custom.Config{
			Name:         customAgent.Name,
			Description:  customAgent.Desc,
			SystemPrompt: customAgent.SystemPrompt,
			Model:        resolveCustomModel(cfg, customAgent),
			ToolNames:    customAgent.Tools,
		})
		if err != nil {
			return fmt.Errorf("创建自定义智能体 %s 失败: %w", customAgent.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	fmt.Printf("本次讨论的主持人: %s\n", moderatorAgent.Name())
	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name())
	}
	fmt.Println(strings.Join(names, ", "))
	return nil
}

// PrintLoopAgentsInfo 打印多智能体讨论模式的智能体信息
func PrintLoopAgentsInfo(ctx context.Context) error {
	teamConfig := config.Get()

	var subAgents []runtimeport.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent, err := discussant.NewAgent(ctx, member)
		if err != nil {
			return fmt.Errorf("创建讨论智能体 %s 失败: %w", member.Name, err)
		}
		subAgents = append(subAgents, agent)
	}

	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name())
	}
	fmt.Println(strings.Join(names, ", "))
	return nil
}

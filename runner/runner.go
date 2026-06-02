// Package runner 提供各工作模式的 Runner 创建工厂函数
package runner

import (
	"context"
	"fkteams/agents"
	agentcommon "fkteams/agents/common"
	"fkteams/agents/custom"
	"fkteams/agents/deep"
	"fkteams/agents/discussant"
	"fkteams/agents/leader"
	"fkteams/agents/moderator"
	"fkteams/agents/tasker"
	"fkteams/common"
	"fkteams/config"
	"fkteams/fkevent"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

var validToolNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// wrapErrorSafe 将子智能体列表中的每个智能体包装为错误安全版本。
func wrapErrorSafe(subAgents []adk.Agent) []adk.Agent {
	wrapped := make([]adk.Agent, len(subAgents))
	for i, agent := range subAgents {
		wrapped[i] = agentcommon.WrapErrorSafe(agent)
	}
	return wrapped
}

type agentToolNameAgent struct {
	inner       adk.Agent
	toolName    string
	displayName string
}

func (a *agentToolNameAgent) Name(context.Context) string {
	return a.toolName
}

func (a *agentToolNameAgent) Description(ctx context.Context) string {
	desc := a.inner.Description(ctx)
	if desc == "" {
		return fmt.Sprintf("指派给 %s 处理任务。", a.displayName)
	}
	return fmt.Sprintf("指派给 %s 处理任务。能力描述：%s", a.displayName, desc)
}

func (a *agentToolNameAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	innerIter := a.inner.Run(ctx, input, opts...)
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	scope := fkevent.MemberScope{
		CallID:   compose.GetToolCallID(ctx),
		ToolName: a.toolName,
		Name:     a.displayName,
	}

	go func() {
		defer gen.Close()
		for {
			event, ok := innerIter.Next()
			if !ok {
				return
			}
			fkevent.RegisterAgentEventScope(event, scope)
			gen.Send(event)
		}
	}()

	return iter
}

func agentToolName(name string, index int, used map[string]bool) string {
	normalized := strings.ToLower(validToolNameChars.ReplaceAllString(name, "_"))
	normalized = strings.Trim(normalized, "_-")
	if normalized == "" || (normalized[0] >= '0' && normalized[0] <= '9') {
		normalized = fmt.Sprintf("member_%d", index+1)
	}
	normalized = "ask_" + normalized

	base := normalized
	for suffix := 2; used[normalized]; suffix++ {
		normalized = fmt.Sprintf("%s_%d", base, suffix)
	}
	used[normalized] = true
	return normalized
}

func buildAgentTools(ctx context.Context, subAgents []adk.Agent) []tool.BaseTool {
	agentTools := make([]tool.BaseTool, 0, len(subAgents))
	usedNames := make(map[string]bool, len(subAgents))
	for i, subAgent := range subAgents {
		displayName := subAgent.Name(ctx)
		wrapped := &agentToolNameAgent{
			inner:       subAgent,
			toolName:    agentToolName(displayName, i, usedNames),
			displayName: displayName,
		}
		fkevent.RegisterAgentToolDisplay(wrapped.toolName, displayName)
		agentTools = append(agentTools, adk.NewAgentTool(ctx, wrapped))
	}
	return agentTools
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

// CreateTeamRunner 创建团队模式 Runner，使用 ChatModelAgent + AgentTool 协作。
func CreateTeamRunner(ctx context.Context) (*adk.Runner, error) {
	agentTools := buildAgentTools(ctx, wrapErrorSafe(agents.GetTeamAgents(ctx)))

	leaderAgent, err := leader.NewAgent(ctx, agentTools...)
	if err != nil {
		return nil, fmt.Errorf("创建 coordinator 智能体失败: %w", err)
	}

	return newRunner(ctx, leaderAgent), nil
}

// CreateDeepAgentsRunner 创建 DeepAgents 模式的 Runner
func CreateDeepAgentsRunner(ctx context.Context) (*adk.Runner, error) {
	subAgents := wrapErrorSafe(agents.GetTeamAgents(ctx))

	deepAgent, err := deep.NewAgent(ctx, subAgents)
	if err != nil {
		return nil, fmt.Errorf("创建 DeepAgents 失败: %w", err)
	}

	return newRunner(ctx, deepAgent), nil
}

// CreateLoopAgentRunner 创建 LoopAgent 模式的 Runner
func CreateLoopAgentRunner(ctx context.Context) (*adk.Runner, error) {
	teamConfig := config.Get()

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent, err := discussant.NewAgent(ctx, member)
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

// CreateCustomRunner 创建自定义会议模式 Runner，使用主持人 ChatModelAgent + AgentTool 协作。
func CreateCustomRunner(ctx context.Context) (*adk.Runner, error) {
	cfg := config.Get()

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent
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

	agentTools := buildAgentTools(ctx, wrapErrorSafe(subAgents))
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

	return newRunner(ctx, moderatorAgent), nil
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

	var moderatorAgent adk.Agent
	var subAgents []adk.Agent
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
	teamConfig := config.Get()

	var subAgents []adk.Agent
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
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))
	return nil
}

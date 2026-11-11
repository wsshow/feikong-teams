// Package agent 提供智能体解析、团队组装和 Runner 创建用例。
package agent

import (
	"context"
	"fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/deep"
	"fkteams/internal/app/agent/catalog/discussant"
	"fkteams/internal/app/agent/catalog/tasker"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/config"
	runtimeport "fkteams/internal/ports/runtime"
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
	runtime, err := runtimeport.RequireAgentToolRuntime(ctx)
	if err != nil {
		return nil, err
	}
	usedNames := make(map[string]bool, len(subAgents))
	var registerDisplay runtimeport.AgentToolDisplayFunc
	if registry, ok := toolmeta.RegistryFromContext(ctx); ok {
		displayNames := agentDisplayNamesByName(ctx)
		registerDisplay = func(toolName, displayName string) {
			if mapped := displayNames[displayName]; mapped != "" {
				displayName = mapped
			}
			registry.RegisterAgentToolDisplay(toolName, displayName)
		}
	}
	return runtime.NewAgentTools(ctx, subAgents, runtimeport.AgentToolConfig{
		ToolName: func(displayName string, index int) string {
			return agentToolName(displayName, index, usedNames)
		},
		RegisterDisplay: registerDisplay,
	})
}

func agentDisplayNamesByName(ctx context.Context) map[string]string {
	infos, err := agents.List(ctx)
	if err != nil {
		return nil
	}
	result := make(map[string]string, len(infos))
	for _, info := range infos {
		if info.Name == "" || info.DisplayName == "" {
			continue
		}
		result[info.Name] = info.DisplayName
	}
	return result
}

// newRunner 用共享配置创建 Runner
func newRunner(ctx context.Context, agent runtimeport.Agent) (runtimeport.Runner, error) {
	runtime, err := runtimeport.RequireRunnerRuntime(ctx)
	if err != nil {
		return nil, err
	}
	return runtime.NewRunner(ctx, runtimeport.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
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

	coordinatorAgent, err := agents.NewBuiltinAgent(ctx, "coordinator", agentTools...)
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
	runtime, err := runtimeport.RequireAgentRuntime(ctx)
	if err != nil {
		return nil, err
	}
	loopAgent, err := runtime.NewLoopAgent(ctx, &runtimeport.LoopAgentConfig{
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

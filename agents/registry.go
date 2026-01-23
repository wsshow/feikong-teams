package agents

import (
	"context"
	"fkteams/agents/analyst"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/summarizer"
	"fkteams/agents/visitor"
	"os"
	"sync"

	"github.com/cloudwego/eino/adk"
)

// AgentInfo 智能体信息
type AgentInfo struct {
	Name        string
	Description string
	Creator     func(ctx context.Context) adk.Agent
}

var (
	// Registry 智能体注册表
	Registry     []AgentInfo
	registryOnce sync.Once
)

// initRegistry 初始化注册表（延迟加载）
func initRegistry() {
	registryOnce.Do(func() {
		ctx := context.Background()

		// 定义所有 agent 的创建函数
		var creators []func() adk.Agent

		// 基础智能体（始终可用）
		creators = append(creators,
			searcher.NewAgent,
			storyteller.NewAgent,
			summarizer.NewAgent,
		)

		// 可选智能体（根据环境变量启用）
		if os.Getenv("FEIKONG_ANALYST_ENABLED") == "true" {
			creators = append(creators, analyst.NewAgent)
		}

		if os.Getenv("FEIKONG_CODER_ENABLED") == "true" {
			creators = append(creators, coder.NewAgent)
		}

		if os.Getenv("FEIKONG_CMDER_ENABLED") == "true" {
			creators = append(creators, cmder.NewAgent)
		}

		if os.Getenv("FEIKONG_SSH_VISITOR_ENABLED") == "true" {
			creators = append(creators, visitor.NewAgent)
		}

		// 动态构建注册表
		Registry = make([]AgentInfo, 0, len(creators))
		for _, creator := range creators {
			agent := creator()
			Registry = append(Registry, AgentInfo{
				Name:        agent.Name(ctx),
				Description: agent.Description(ctx),
				Creator: func(cachedAgent adk.Agent) func(ctx context.Context) adk.Agent {
					return func(ctx context.Context) adk.Agent {
						return cachedAgent
					}
				}(agent),
			})
		}
	})
}

// GetRegistry 获取智能体注册表
func GetRegistry() []AgentInfo {
	initRegistry()
	return Registry
}

// GetAgentByName 根据名字获取智能体
func GetAgentByName(name string) *AgentInfo {
	initRegistry()
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

// GetTeamAgents 获取团队模式的智能体列表
func GetTeamAgents(ctx context.Context) []adk.Agent {
	initRegistry()

	var subAgents []adk.Agent
	for _, agentInfo := range Registry {
		subAgents = append(subAgents, agentInfo.Creator(ctx))
	}

	return subAgents
}

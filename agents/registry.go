package agents

import (
	"context"
	"fkteams/agents/analyst"
	assistantagent "fkteams/agents/assistant"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/custom"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/summarizer"
	"fkteams/agents/visitor"
	"fkteams/config"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/pterm/pterm"
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
		cfg := config.Get()

		type agentCreator struct {
			name    string
			creator func(ctx context.Context) (adk.Agent, error)
		}

		// 基础智能体（始终可用）
		creators := []agentCreator{
			{"cmder", cmder.NewAgent},
			{"coder", coder.NewAgent},
			{"storyteller", storyteller.NewAgent},
			{"summarizer", summarizer.NewAgent},
		}

		// 可选智能体（根据配置文件启用）
		if cfg.Agents.Searcher {
			creators = append(creators, agentCreator{"searcher", searcher.NewAgent})
		}

		if cfg.Agents.Analyst {
			creators = append(creators, agentCreator{"analyst", analyst.NewAgent})
		}

		if cfg.Agents.SSHVisitor.Enabled {
			creators = append(creators, agentCreator{"visitor", visitor.NewAgent})
		}

		if cfg.Agents.Assistant {
			creators = append(creators, agentCreator{"assistant", assistantagent.NewAgent})
		}

		// 动态构建注册表
		Registry = make([]AgentInfo, 0, len(creators))
		for _, c := range creators {
			agent, err := c.creator(ctx)
			if err != nil {
				pterm.Warning.Printfln("初始化智能体 %s 失败: %v", c.name, err)
				continue
			}
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

		// 加载配置文件中的自定义智能体
		loadCustomAgents(ctx)
	})
}

// loadCustomAgents 从配置文件加载自定义智能体并添加到注册表
func loadCustomAgents(ctx context.Context) {
	cfg := config.Get()

	if len(cfg.Custom.Agents) == 0 {
		return
	}

	existingNames := make(map[string]bool, len(Registry))
	for _, info := range Registry {
		existingNames[info.Name] = true
	}

	for _, agentCfg := range cfg.Custom.Agents {
		if agentCfg.Name == "" {
			continue
		}

		if existingNames[agentCfg.Name] {
			pterm.Warning.Printfln("自定义智能体 \"%s\" 与已有智能体名称重复，不建议使用相同名称", agentCfg.Name)
		}

		mc := cfg.ResolveModel(agentCfg.Model)
		var model custom.Model
		if mc != nil {
			model = custom.Model{
				Provider: mc.Provider,
				Name:     mc.Model,
				APIKey:   mc.APIKey,
				BaseURL:  mc.BaseURL,
			}
		}

		agent, err := custom.NewAgent(ctx, custom.Config{
			Name:         agentCfg.Name,
			Description:  agentCfg.Desc,
			SystemPrompt: agentCfg.SystemPrompt,
			Model:        model,
			ToolNames:    agentCfg.Tools,
		})
		if err != nil {
			pterm.Warning.Printfln("初始化自定义智能体 \"%s\" 失败: %v", agentCfg.Name, err)
			continue
		}

		Registry = append(Registry, AgentInfo{
			Name:        agentCfg.Name,
			Description: agentCfg.Desc,
			Creator: func(cachedAgent adk.Agent) func(ctx context.Context) adk.Agent {
				return func(ctx context.Context) adk.Agent {
					return cachedAgent
				}
			}(agent),
		})
	}
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

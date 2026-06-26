package agents

import (
	"context"
	"fkteams/internal/app/agent/catalog/analyst"
	assistantagent "fkteams/internal/app/agent/catalog/assistant"
	"fkteams/internal/app/agent/catalog/coder"
	"fkteams/internal/app/agent/catalog/custom"
	"fkteams/internal/app/agent/catalog/researcher"
	"fkteams/internal/app/agent/catalog/visitor"
	"fkteams/internal/app/config"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"log"
	"sync"
)

// AgentInfo 智能体信息。
type AgentInfo struct {
	Name        string
	Description string
	Aliases     []string
	Creator     func(ctx context.Context) (runtimeport.Agent, error)
}

type registryContextKey struct{}

// Registry 持有当前进程可用智能体目录。
type Registry struct {
	mu     sync.RWMutex
	loaded bool
	agents []AgentInfo
}

// NewRegistry 创建惰性加载的智能体目录。
func NewRegistry() *Registry {
	return &Registry{}
}

// WithRegistry 将智能体目录注入上下文。
func WithRegistry(ctx context.Context, registry *Registry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if registry == nil {
		return ctx
	}
	return context.WithValue(ctx, registryContextKey{}, registry)
}

// RegistryFromContext 从上下文读取智能体目录。
func RegistryFromContext(ctx context.Context) (*Registry, bool) {
	if ctx == nil {
		return nil, false
	}
	registry, ok := ctx.Value(registryContextKey{}).(*Registry)
	return registry, ok && registry != nil
}

// RequireRegistry 从上下文读取智能体目录，缺失时返回明确错误。
func RequireRegistry(ctx context.Context) (*Registry, error) {
	if registry, ok := RegistryFromContext(ctx); ok {
		return registry, nil
	}
	return nil, fmt.Errorf("agent registry is not configured")
}

// List 返回当前智能体目录快照。
func (r *Registry) List() []AgentInfo {
	if r == nil {
		return nil
	}
	r.ensureLoaded()
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AgentInfo, len(r.agents))
	copy(result, r.agents)
	return result
}

// AgentByName 根据名称或别名查找智能体。
func (r *Registry) AgentByName(name string) *AgentInfo {
	if r == nil {
		return nil
	}
	r.ensureLoaded()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.agents {
		if r.agents[i].Name == name {
			return cloneAgentInfo(r.agents[i])
		}
		for _, alias := range r.agents[i].Aliases {
			if alias == name {
				return cloneAgentInfo(r.agents[i])
			}
		}
	}
	return nil
}

// TeamAgents 创建团队模式成员智能体。
func (r *Registry) TeamAgents(ctx context.Context) ([]runtimeport.Agent, error) {
	if r == nil {
		return nil, fmt.Errorf("agent registry is nil")
	}
	r.ensureLoaded()
	infos := r.List()
	subAgents := make([]runtimeport.Agent, 0, len(infos))
	for _, agentInfo := range infos {
		agent, err := agentInfo.Creator(ctx)
		if err != nil {
			return nil, err
		}
		subAgents = append(subAgents, agent)
	}
	return subAgents, nil
}

// Reload 重新构建智能体目录。
func (r *Registry) Reload() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.agents = buildRegistry()
	r.loaded = true
	r.mu.Unlock()
}

func (r *Registry) ensureLoaded() {
	r.mu.RLock()
	loaded := r.loaded
	r.mu.RUnlock()
	if loaded {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded {
		return
	}
	r.agents = buildRegistry()
	r.loaded = true
}

// List 返回上下文中的智能体目录快照。
func List(ctx context.Context) ([]AgentInfo, error) {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.List(), nil
}

// AgentByName 根据上下文中的目录查找智能体。
func AgentByName(ctx context.Context, name string) (*AgentInfo, error) {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.AgentByName(name), nil
}

// TeamAgents 创建上下文中目录的团队成员。
func TeamAgents(ctx context.Context) ([]runtimeport.Agent, error) {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	return registry.TeamAgents(ctx)
}

// Reload 重新加载上下文中的智能体目录。
func Reload(ctx context.Context) error {
	registry, err := RequireRegistry(ctx)
	if err != nil {
		return err
	}
	registry.Reload()
	return nil
}

func buildRegistry() []AgentInfo {
	cfg := config.Get()

	type agentCreator struct {
		name        string
		description string
		aliases     []string
		creator     func(ctx context.Context) (runtimeport.Agent, error)
	}

	creators := []agentCreator{
		{name: "coder", description: "软件工程师，负责代码实现、调试、重构和工程验证。", aliases: []string{"小码"}, creator: coder.NewAgent},
	}

	if cfg.Agents.Researcher {
		creators = append(creators, agentCreator{name: "researcher", description: "网络研究员，负责检索、抓取、交叉验证和整理时效信息。", aliases: []string{"小搜"}, creator: researcher.NewAgent})
	}
	if cfg.Agents.Analyst {
		creators = append(creators, agentCreator{name: "analyst", description: "数据分析师，负责使用表格、脚本和文档工具提取洞察。", aliases: []string{"小析"}, creator: analyst.NewAgent})
	}
	if cfg.Agents.SSHVisitor.Enabled {
		creators = append(creators, agentCreator{name: "remote", description: "远程运维专家，负责通过 SSH 管理服务器、执行命令和传输文件。", aliases: []string{"小访", "visitor"}, creator: visitor.NewAgent})
	}
	if cfg.Agents.Assistant {
		creators = append(creators, agentCreator{name: "generalist", description: "通用执行助手，负责综合命令、文件、搜索和文档工具完成开放任务。", aliases: []string{"小助", "assistant"}, creator: assistantagent.NewAgent})
	}

	entries := make([]AgentInfo, 0, len(creators)+len(cfg.Custom.Agents))
	for _, c := range creators {
		entries = append(entries, AgentInfo{
			Name:        c.name,
			Description: c.description,
			Aliases:     append([]string(nil), c.aliases...),
			Creator:     c.creator,
		})
	}
	return appendCustomAgents(entries, cfg)
}

func appendCustomAgents(entries []AgentInfo, cfg *config.Config) []AgentInfo {
	if len(cfg.Custom.Agents) == 0 {
		return entries
	}

	existingNames := make(map[string]bool, len(entries))
	for _, info := range entries {
		existingNames[info.Name] = true
	}

	for _, agentCfg := range cfg.Custom.Agents {
		if agentCfg.Name == "" {
			continue
		}
		if existingNames[agentCfg.Name] {
			log.Printf("[agent] 自定义智能体 %q 与已有智能体名称重复，不建议使用相同名称", agentCfg.Name)
		}

		agentCfg := agentCfg
		entries = append(entries, AgentInfo{
			Name:        agentCfg.Name,
			Description: agentCfg.Desc,
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				mc := config.Get().ResolveModel(agentCfg.Model)
				var model custom.Model
				if mc != nil {
					model = custom.Model{
						Provider: mc.Provider,
						Name:     mc.Model,
						APIKey:   mc.APIKey,
						BaseURL:  mc.BaseURL,
					}
				}
				return custom.NewAgent(ctx, custom.Config{
					Name:         agentCfg.Name,
					Description:  agentCfg.Desc,
					SystemPrompt: agentCfg.SystemPrompt,
					Model:        model,
					ToolNames:    agentCfg.Tools,
				})
			},
		})
	}
	return entries
}

func cloneAgentInfo(info AgentInfo) *AgentInfo {
	info.Aliases = append([]string(nil), info.Aliases...)
	return &info
}

package agents

import (
	"context"
	"fkteams/internal/app/agent/catalog/analyst"
	assistantagent "fkteams/internal/app/agent/catalog/assistant"
	"fkteams/internal/app/agent/catalog/coder"
	"fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/agent/catalog/coordinator"
	"fkteams/internal/app/agent/catalog/custom"
	"fkteams/internal/app/agent/catalog/researcher"
	"fkteams/internal/app/agent/catalog/visitor"
	"fkteams/internal/app/config"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
	"fmt"
	"sync"
)

// AgentInfo 智能体信息。
type AgentInfo struct {
	Name        string
	DisplayName string
	Description string
	Aliases     []string
	Builtin     bool
	TeamMember  bool
	Enabled     bool
	Prompt      string
	ModelID     string
	ToolNames   []string
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
	for i := range r.agents {
		result[i] = *cloneAgentInfo(r.agents[i])
	}
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
		if !agentInfo.TeamMember {
			continue
		}
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

// NewBuiltinAgent 按当前配置创建内置智能体，并允许注入额外工具。
func NewBuiltinAgent(ctx context.Context, id string, extraTools ...runtimeport.Tool) (runtimeport.Agent, error) {
	cfg := config.Get()
	for _, item := range ConfigItems(cfg) {
		if item.ID != id {
			continue
		}
		if !item.Enabled {
			return nil, fmt.Errorf("agent %s is disabled", id)
		}
		for _, spec := range builtinAgentSpecs() {
			if spec.id != id {
				continue
			}
			def := spec.definition(cfg)
			applyAgentConfig(&def, item)
			def.Tools = append(def.Tools, extraTools...)
			return buildConfiguredDefinition(ctx, cfg, def, item)
		}
		return nil, fmt.Errorf("agent %s is not builtin", id)
	}
	return nil, fmt.Errorf("agent %s not found", id)
}

func buildRegistry() []AgentInfo {
	cfg := config.Get()
	configs := ConfigItems(cfg)
	entries := make([]AgentInfo, 0, len(configs))
	for _, agentCfg := range configs {
		if !agentCfg.Enabled {
			continue
		}
		info := agentInfoFromConfig(cfg, agentCfg)
		if info.Name == "" || info.Creator == nil {
			continue
		}
		entries = append(entries, info)
	}
	return entries
}

type builtinAgentSpec struct {
	id               string
	displayName      string
	aliases          []string
	teamMember       bool
	enabledByDefault bool
	definition       func(*config.Config) common.Definition
}

func builtinAgentSpecs() []builtinAgentSpec {
	return []builtinAgentSpec{
		{
			id:               "coordinator",
			displayName:      "协调者",
			aliases:          []string{"小队长"},
			enabledByDefault: true,
			definition: func(*config.Config) common.Definition {
				return coordinator.DefaultDefinition()
			},
		},
		{
			id:               "coder",
			displayName:      "代码工程师",
			aliases:          []string{"小码"},
			teamMember:       true,
			enabledByDefault: true,
			definition: func(*config.Config) common.Definition {
				return coder.DefaultDefinition()
			},
		},
		{
			id:               "researcher",
			displayName:      "研究员",
			aliases:          []string{"小搜"},
			teamMember:       true,
			enabledByDefault: true,
			definition: func(*config.Config) common.Definition {
				return researcher.DefaultDefinition()
			},
		},
		{
			id:          "analyst",
			displayName: "数据分析师",
			aliases:     []string{"小析"},
			teamMember:  true,
			definition: func(*config.Config) common.Definition {
				return analyst.DefaultDefinition()
			},
		},
		{
			id:          "remote",
			displayName: "远程运维",
			aliases:     []string{"小访", "visitor"},
			teamMember:  true,
			definition: func(*config.Config) common.Definition {
				return visitor.DefaultDefinition("", "")
			},
		},
		{
			id:          "generalist",
			displayName: "通用助手",
			aliases:     []string{"小助", "assistant"},
			teamMember:  true,
			definition: func(*config.Config) common.Definition {
				return assistantagent.DefaultDefinition()
			},
		},
	}
}

func ConfigItems(cfg *config.Config) []config.AgentConfig {
	if cfg == nil {
		cfg = config.Get()
	}
	overrides := make(map[string]config.AgentConfig, len(cfg.Agents.Items))
	for _, item := range cfg.Agents.Items {
		if item.ID == "" {
			continue
		}
		overrides[item.ID] = item
	}

	result := make([]config.AgentConfig, 0, len(builtinAgentSpecs())+len(cfg.Agents.Items))
	seen := make(map[string]bool, len(cfg.Agents.Items))
	for _, spec := range builtinAgentSpecs() {
		def := spec.definition(cfg)
		item := config.AgentConfig{
			ID:          spec.id,
			Name:        spec.displayName,
			Description: def.Description,
			Prompt:      def.Instruction,
			Tools:       append([]string(nil), def.ToolNames...),
			Enabled:     spec.enabledByDefault,
			Builtin:     true,
			TeamMember:  spec.teamMember,
		}
		if override, ok := overrides[spec.id]; ok {
			item = mergeAgentConfig(item, override)
		}
		result = append(result, item)
		seen[spec.id] = true
	}
	for _, item := range cfg.Agents.Items {
		if item.ID == "" || seen[item.ID] {
			continue
		}
		item = cloneAgentConfig(item)
		item.Builtin = false
		item.TeamMember = true
		result = append(result, item)
		seen[item.ID] = true
	}
	return result
}

func mergeAgentConfig(base, override config.AgentConfig) config.AgentConfig {
	base.ID = override.ID
	base.Enabled = override.Enabled
	if override.Name != "" {
		base.Name = override.Name
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.Prompt != "" {
		base.Prompt = override.Prompt
	}
	if override.ModelID != "" {
		base.ModelID = override.ModelID
	}
	if override.Tools != nil {
		base.Tools = append([]string(nil), override.Tools...)
	}
	if override.SSH != nil {
		ssh := *override.SSH
		base.SSH = &ssh
	}
	return base
}

func agentInfoFromConfig(cfg *config.Config, agentCfg config.AgentConfig) AgentInfo {
	for _, spec := range builtinAgentSpecs() {
		if spec.id != agentCfg.ID {
			continue
		}
		def := spec.definition(cfg)
		applyAgentConfig(&def, agentCfg)
		return AgentInfo{
			Name:        agentCfg.ID,
			DisplayName: agentCfg.Name,
			Description: agentCfg.Description,
			Aliases:     append([]string(nil), spec.aliases...),
			Builtin:     true,
			TeamMember:  spec.teamMember,
			Enabled:     agentCfg.Enabled,
			Prompt:      def.Instruction,
			ModelID:     agentCfg.ModelID,
			ToolNames:   append([]string(nil), def.ToolNames...),
			Creator: func(ctx context.Context) (runtimeport.Agent, error) {
				return buildConfiguredDefinition(ctx, cfg, def, agentCfg)
			},
		}
	}
	return customAgentInfo(cfg, agentCfg)
}

func applyAgentConfig(def *common.Definition, cfg config.AgentConfig) {
	def.Name = cfg.ID
	if cfg.Description != "" {
		def.Description = cfg.Description
	}
	if cfg.Prompt != "" {
		def.Instruction = cfg.Prompt
	}
	if cfg.Tools != nil {
		def.ToolNames = append([]string(nil), cfg.Tools...)
	}
}

func buildConfiguredDefinition(ctx context.Context, cfg *config.Config, def common.Definition, agentCfg config.AgentConfig) (runtimeport.Agent, error) {
	if agentCfg.SSH != nil {
		applySSHConfigToDefinition(&def, agentCfg.SSH)
		ctx = apptools.WithResolveContextPatch(ctx, apptools.ToolResolveContext{
			SSH: &apptools.SSHConfig{
				Host:     agentCfg.SSH.Host,
				Username: agentCfg.SSH.Username,
				Password: agentCfg.SSH.Password,
			},
		})
	}
	if agentCfg.ModelID != "" {
		modelCfg := cfg.ResolveModel(agentCfg.ModelID)
		if modelCfg == nil {
			return nil, fmt.Errorf("model_id %q not found for agent %s", agentCfg.ModelID, agentCfg.ID)
		}
		chatModel, err := common.NewChatModelWithConfig(ctx, &modelregistry.Config{
			Provider: modelregistry.Type(modelCfg.Provider),
			APIKey:   modelCfg.APIKey,
			BaseURL:  modelCfg.BaseURL,
			Model:    modelCfg.Model,
		})
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
		def.Model = chatModel
	}
	return common.BuildAgent(ctx, def)
}

func applySSHConfigToDefinition(def *common.Definition, ssh *config.AgentSSH) {
	if def == nil || ssh == nil {
		return
	}
	if def.TemplateVars == nil {
		def.TemplateVars = make(map[string]any)
	}
	def.TemplateVars["ssh_host"] = ssh.Host
	def.TemplateVars["ssh_username"] = ssh.Username
}

func cloneAgentConfig(item config.AgentConfig) config.AgentConfig {
	item.Tools = append([]string(nil), item.Tools...)
	if item.SSH != nil {
		ssh := *item.SSH
		item.SSH = &ssh
	}
	return item
}

func customAgentInfo(cfg *config.Config, agentCfg config.AgentConfig) AgentInfo {
	agentID := agentCfg.ID
	if agentID == "" {
		agentID = agentCfg.Name
	}
	if agentID == "" || agentCfg.Name == "" {
		return AgentInfo{}
	}
	agentCfg.ID = agentID
	return AgentInfo{
		Name:        agentID,
		DisplayName: agentCfg.Name,
		Description: agentCfg.Description,
		TeamMember:  true,
		Enabled:     agentCfg.Enabled,
		Prompt:      agentCfg.Prompt,
		ModelID:     agentCfg.ModelID,
		ToolNames:   append([]string(nil), agentCfg.Tools...),
		Creator: func(ctx context.Context) (runtimeport.Agent, error) {
			if agentCfg.SSH != nil {
				ctx = apptools.WithResolveContextPatch(ctx, apptools.ToolResolveContext{
					SSH: &apptools.SSHConfig{
						Host:     agentCfg.SSH.Host,
						Username: agentCfg.SSH.Username,
						Password: agentCfg.SSH.Password,
					},
				})
			}
			return custom.NewAgent(ctx, custom.Config{
				Name:        agentID,
				Description: agentCfg.Description,
				Prompt:      agentCfg.Prompt,
				Model:       resolveCustomModel(cfg, agentCfg),
				ToolNames:   agentCfg.Tools,
			})
		},
	}
}

func resolveCustomModel(cfg *config.Config, agent config.AgentConfig) custom.Model {
	mc := cfg.ResolveModel(agent.ModelID)
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

func cloneAgentInfo(info AgentInfo) *AgentInfo {
	info.Aliases = append([]string(nil), info.Aliases...)
	info.ToolNames = append([]string(nil), info.ToolNames...)
	return &info
}

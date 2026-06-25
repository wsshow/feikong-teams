package agent

import (
	"context"
	"fkteams/agents"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"sync"
)

const (
	ModeTeam       = "team"
	ModeSupervisor = "supervisor"
	ModeRoundtable = "roundtable"
	ModeCustom     = "custom"
	ModeDeep       = "deep"
)

// Cache 负责按模式或智能体名称复用 Runner。
type Cache struct {
	mu    sync.RWMutex
	items map[string]runtimeport.Runner
}

// NewCache 创建一个 Runner 缓存。
func NewCache() *Cache {
	return &Cache{items: make(map[string]runtimeport.Runner)}
}

// GetOrCreate 获取缓存项，不存在时调用 factory 创建。
func (c *Cache) GetOrCreate(key string, factory func() (runtimeport.Runner, error)) (runtimeport.Runner, error) {
	c.mu.RLock()
	if r, exists := c.items[key]; exists {
		c.mu.RUnlock()
		return r, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if r, exists := c.items[key]; exists {
		return r, nil
	}

	r, err := factory()
	if err != nil {
		return nil, err
	}

	c.items[key] = r
	return r, nil
}

// Clear 清空缓存，使后续请求重新创建 Runner。
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]runtimeport.Runner)
}

// Resolve 按模式或智能体名称获取 Runner，未知模式会尝试按智能体名称解析。
func (c *Cache) Resolve(ctx context.Context, mode, agentName string) (runtimeport.Runner, error) {
	key, factory, err := resolveFactory(ctx, mode, agentName, false)
	if err != nil {
		return nil, err
	}
	return c.GetOrCreate(key, factory)
}

// ResolveWithTeamFallback 保留 Web 入口的兼容行为：未知模式回退到团队模式。
func (c *Cache) ResolveWithTeamFallback(ctx context.Context, mode, agentName string) (runtimeport.Runner, error) {
	key, factory, err := resolveFactory(ctx, mode, agentName, true)
	if err != nil {
		return nil, err
	}
	return c.GetOrCreate(key, factory)
}

// Resolve 创建一次性的 Runner，不使用缓存。
func Resolve(ctx context.Context, mode, agentName string) (runtimeport.Runner, error) {
	_, factory, err := resolveFactory(ctx, mode, agentName, false)
	if err != nil {
		return nil, err
	}
	return factory()
}

func resolveFactory(ctx context.Context, mode, agentName string, fallbackToTeam bool) (string, func() (runtimeport.Runner, error), error) {
	if agentName != "" {
		return agentCacheKey(agentName), func() (runtimeport.Runner, error) {
			return createAgentRunnerByName(ctx, agentName)
		}, nil
	}

	if mode == "" {
		mode = ModeTeam
	}

	switch mode {
	case ModeRoundtable:
		return mode, func() (runtimeport.Runner, error) {
			return CreateLoopAgentRunner(ctx)
		}, nil
	case ModeCustom:
		return mode, func() (runtimeport.Runner, error) {
			return CreateCustomRunner(ctx)
		}, nil
	case ModeDeep:
		return mode, func() (runtimeport.Runner, error) {
			return CreateDeepAgentsRunner(ctx)
		}, nil
	case ModeTeam, ModeSupervisor:
		return mode, func() (runtimeport.Runner, error) {
			return CreateTeamRunner(ctx)
		}, nil
	default:
		if fallbackToTeam {
			return ModeTeam, func() (runtimeport.Runner, error) {
				return CreateTeamRunner(ctx)
			}, nil
		}
		info := agents.GetAgentByName(mode)
		if info == nil {
			return "", nil, fmt.Errorf("unknown mode or agent: %s", mode)
		}
		return agentCacheKey(mode), func() (runtimeport.Runner, error) {
			agent, err := info.Creator(ctx)
			if err != nil {
				return nil, fmt.Errorf("create agent %s: %w", mode, err)
			}
			return CreateAgentRunner(ctx, agent)
		}, nil
	}
}

func createAgentRunnerByName(ctx context.Context, agentName string) (runtimeport.Runner, error) {
	agentInfo := agents.GetAgentByName(agentName)
	if agentInfo == nil {
		return nil, fmt.Errorf("agent not found: %s", agentName)
	}
	agent, err := agentInfo.Creator(ctx)
	if err != nil {
		return nil, fmt.Errorf("create agent %s: %w", agentName, err)
	}
	return CreateAgentRunner(ctx, agent)
}

func agentCacheKey(agentName string) string {
	return "agent_" + agentName
}

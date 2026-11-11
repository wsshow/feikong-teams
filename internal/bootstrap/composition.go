// Package bootstrap 负责装配应用入口共享的进程级依赖。
package bootstrap

import (
	"context"
	"fmt"
	"time"

	javascripthooks "fkteams/internal/adapters/hooks/javascript"
	modelproviders "fkteams/internal/adapters/model/providers"
	mcpadapter "fkteams/internal/adapters/tools/mcp"
	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/config"
	"fkteams/internal/app/execution"
	bootstrapruntimes "fkteams/internal/bootstrap/runtimes"
	bootstraptools "fkteams/internal/bootstrap/tools"
	"fkteams/internal/runtime/hooks"
)

// ExecutionDependencies 保存一次应用实例所需的执行依赖。
// 各注册表均为实例所有，避免入口依赖可变的进程级全局状态。
type ExecutionDependencies struct {
	execution.Dependencies
	ExecutionGate         *config.ExecutionGate
	ModelProviderRegistry *modelproviders.Registry
}

// NewExecutionDependencies 创建并连接默认 runtime、模型、工具和智能体注册表。
func NewExecutionDependencies() (*ExecutionDependencies, error) {
	mcpProvider := mcpadapter.NewProvider()
	runtimeDefaults, err := bootstrapruntimes.NewDefaults(bootstrapruntimes.Options{
		MCPProvider: mcpProvider,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize runtime dependencies: %w", err)
	}

	toolRegistry, err := bootstraptools.RegisterDefaults(mcpProvider)
	if err != nil {
		return nil, fmt.Errorf("register default tools: %w", err)
	}
	hookBus := hooks.NewBus()
	hookBus.Register(javascripthooks.NewHandler(func() []config.JavaScriptHook {
		return config.Get().JavaScript.Hooks
	}), hooks.Options{
		Timeout:     30 * time.Second,
		ErrorPolicy: hooks.ErrorFail,
		Concurrent:  true,
	})

	return &ExecutionDependencies{
		ExecutionGate:         &config.ExecutionGate{},
		ModelProviderRegistry: runtimeDefaults.ModelProviderRegistry,
		Dependencies: execution.Dependencies{
			Runtime:       runtimeDefaults.Runtime,
			Interrupt:     runtimeDefaults.Interrupt,
			ModelRegistry: runtimeDefaults.ModelRegistry,
			ToolRegistry:  toolRegistry,
			ToolDisplays:  toolmeta.NewRegistry(),
			AgentRegistry: agents.NewRegistry(),
			HookBus:       hookBus,
		},
	}, nil
}

// Context 将执行依赖绑定到父 context，供传输层和应用用例按需读取。
func (d *ExecutionDependencies) Context(parent context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if d == nil {
		return parent
	}

	return modelproviders.WithRegistry(d.Dependencies.Context(parent), d.ModelProviderRegistry)
}

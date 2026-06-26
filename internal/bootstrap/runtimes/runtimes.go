package runtimes

import (
	"sync"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	einoengine "fkteams/internal/adapters/runtime/eino/engine"
	toolmcp "fkteams/internal/adapters/tools/mcp"
	runtimeport "fkteams/internal/ports/runtime"
	toolport "fkteams/internal/ports/tools"
	runtimeregistry "fkteams/internal/runtime/registry"

	_ "fkteams/internal/adapters/runtime/eino/providers/register"
)

var (
	registerOnce sync.Once
	registerErr  error
)

func init() {
	_ = RegisterDefaults()
}

// RegisterDefaults 注册默认 runtime adapter 和关联桥接能力。
func RegisterDefaults() error {
	registerOnce.Do(func() {
		registerErr = registerDefaults()
	})
	return registerErr
}

func registerDefaults() error {
	engine := einoengine.NewEngine()
	if err := runtimeregistry.Register(runtimeregistry.DefaultRuntimeName, engine); err != nil {
		return err
	}
	if provider, ok := any(engine).(toolport.MCPClientToolProvider); ok {
		toolmcp.RegisterToolProvider(provider.MCPTools)
	}
	runtimeport.RegisterInterruptRuntime(einoruntime.NewInterruptRuntime())
	return nil
}

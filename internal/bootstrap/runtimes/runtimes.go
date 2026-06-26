package runtimes

import (
	"sync"

	einoruntime "fkteams/internal/adapters/runtime/eino"
	einoengine "fkteams/internal/adapters/runtime/eino/engine"
	toolmcp "fkteams/internal/adapters/tools/mcp"
	runtimeport "fkteams/internal/ports/runtime"
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

// DefaultEngine 返回组合根注册的默认 runtime engine。
func DefaultEngine() (runtimeport.Engine, error) {
	if err := RegisterDefaults(); err != nil {
		return nil, err
	}
	return runtimeregistry.Engine()
}

func registerDefaults() error {
	engine := einoengine.NewEngine()
	if err := runtimeregistry.Register(runtimeregistry.DefaultRuntimeName, engine); err != nil {
		return err
	}
	toolmcp.RegisterToolProvider(engine.MCPTools)
	runtimeport.RegisterInterruptRuntime(einoruntime.NewInterruptRuntime())
	return nil
}

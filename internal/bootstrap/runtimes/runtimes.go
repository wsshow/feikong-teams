package runtimes

import (
	"fmt"
	"sync"

	modelproviders "fkteams/internal/adapters/model/providers"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	einoengine "fkteams/internal/adapters/runtime/eino/engine"
	einoproviders "fkteams/internal/adapters/runtime/eino/providers/register"
	toolmcp "fkteams/internal/adapters/tools/mcp"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
	runtimeregistry "fkteams/internal/runtime/registry"
)

var (
	registerOnce sync.Once
	registerErr  error
	modelReg     *modelregistry.Registry
	providerReg  *modelproviders.Registry
)

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

// DefaultModelRegistry 返回组合根注册的模型工厂注册表。
func DefaultModelRegistry() (*modelregistry.Registry, error) {
	if err := RegisterDefaults(); err != nil {
		return nil, err
	}
	if modelReg == nil {
		return nil, fmt.Errorf("model registry is not registered")
	}
	return modelReg, nil
}

// DefaultModelProviderRegistry 返回组合根注册的模型 provider 注册表。
func DefaultModelProviderRegistry() (*modelproviders.Registry, error) {
	if err := RegisterDefaults(); err != nil {
		return nil, err
	}
	if providerReg == nil {
		return nil, fmt.Errorf("model provider registry is not registered")
	}
	return providerReg, nil
}

// DefaultInterruptRuntime 返回默认 HITL 中断 runtime。
func DefaultInterruptRuntime() runtimeport.InterruptRuntime {
	return einoruntime.NewInterruptRuntime()
}

func registerDefaults() error {
	providerReg = modelproviders.NewRegistry()
	modelReg = modelregistry.NewRegistry()
	einoproviders.RegisterDefaults(providerReg, modelReg)
	engine := einoengine.NewEngine()
	if err := runtimeregistry.Register(runtimeregistry.DefaultRuntimeName, engine); err != nil {
		return err
	}
	toolmcp.RegisterToolProvider(engine.MCPTools)
	return nil
}

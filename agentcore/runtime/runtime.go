package runtime

import (
	"fkteams/agentcore"
	toolmcp "fkteams/tools/mcp"
	"fmt"
	"sort"
	"sync"
)

const DefaultRuntimeName = "eino"

var registry = struct {
	sync.RWMutex
	defaultName string
	engines     map[string]agentcore.Engine
}{
	defaultName: DefaultRuntimeName,
	engines:     make(map[string]agentcore.Engine),
}

func Engine() agentcore.Engine {
	engine, err := EngineByName(DefaultName())
	if err != nil {
		panic(err)
	}
	return engine
}

func Register(name string, engine agentcore.Engine) {
	if name == "" {
		panic("runtime name is empty")
	}
	if engine == nil {
		panic("runtime engine is nil")
	}
	registry.Lock()
	registry.engines[name] = engine
	registry.Unlock()

	if provider, ok := engine.(agentcore.MCPToolProvider); ok {
		toolmcp.RegisterToolProvider(provider.MCPTools)
	}
}

func Use(name string) error {
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.engines[name]; !ok {
		return fmt.Errorf("runtime %s is not registered", name)
	}
	registry.defaultName = name
	return nil
}

func DefaultName() string {
	registry.RLock()
	defer registry.RUnlock()
	return registry.defaultName
}

func EngineByName(name string) (agentcore.Engine, error) {
	registry.RLock()
	defer registry.RUnlock()
	engine, ok := registry.engines[name]
	if !ok {
		return nil, fmt.Errorf("runtime %s is not registered", name)
	}
	return engine, nil
}

func RegisteredNames() []string {
	registry.RLock()
	defer registry.RUnlock()
	names := make([]string, 0, len(registry.engines))
	for name := range registry.engines {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

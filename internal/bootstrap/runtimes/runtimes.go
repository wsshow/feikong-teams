package runtimes

import (
	einoengine "fkteams/internal/adapters/runtime/eino/engine"
	toolmcp "fkteams/internal/app/tools/mcp"
	runtimeport "fkteams/internal/ports/runtime"
	runtimeregistry "fkteams/internal/runtime/registry"

	_ "fkteams/internal/adapters/runtime/eino/providers/register"
)

func init() {
	engine := einoengine.NewEngine()
	runtimeregistry.Register(runtimeregistry.DefaultRuntimeName, engine)
	if provider, ok := any(engine).(runtimeport.MCPToolProvider); ok {
		toolmcp.RegisterToolProvider(provider.MCPTools)
	}
}

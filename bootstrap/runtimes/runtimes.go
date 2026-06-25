package runtimes

import (
	agentruntime "fkteams/agentcore/runtime"
	einoengine "fkteams/internal/adapters/runtime/eino/engine"

	_ "fkteams/internal/adapters/runtime/eino/providers/register"
)

func init() {
	agentruntime.Register(agentruntime.DefaultRuntimeName, einoengine.NewEngine())
}

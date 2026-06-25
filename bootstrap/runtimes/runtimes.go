package runtimes

import (
	einoengine "fkteams/agentcore/eino/engine"
	agentruntime "fkteams/agentcore/runtime"

	_ "fkteams/agentcore/eino/providers/register"
)

func init() {
	agentruntime.Register(agentruntime.DefaultRuntimeName, einoengine.NewEngine())
}

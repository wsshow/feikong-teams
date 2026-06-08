package runtime

import (
	"fkteams/agentcore"
	einoengine "fkteams/agentcore/eino/engine"
	toolmcp "fkteams/tools/mcp"

	_ "fkteams/agentcore/eino/providers/register"
)

var defaultEinoEngine = einoengine.NewEngine()
var defaultEngine agentcore.Engine = defaultEinoEngine

func init() {
	if provider, ok := any(defaultEinoEngine).(agentcore.MCPToolProvider); ok {
		toolmcp.RegisterToolProvider(provider.MCPTools)
	}
}

func Engine() agentcore.Engine {
	return defaultEngine
}

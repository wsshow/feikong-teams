package agentruntime

import (
	"fkteams/agentcore"
	einoengine "fkteams/agentcore/eino/engine"
	toolmcp "fkteams/tools/mcp"

	_ "fkteams/agentcore/eino/providers/register"
)

var defaultEinoEngine = einoengine.NewEngine()
var defaultEngine agentcore.Engine = defaultEinoEngine

func init() {
	toolmcp.RegisterToolProvider(defaultEinoEngine.MCPTools)
}

func Engine() agentcore.Engine {
	return defaultEngine
}

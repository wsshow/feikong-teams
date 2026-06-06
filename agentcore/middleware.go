package agentcore

type AgentMiddleware interface {
	Name() string
}

type ToolMiddleware interface {
	Name() string
}

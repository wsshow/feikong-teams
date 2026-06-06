package eino

import (
	"fkteams/agentcore"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

type agentMiddleware struct {
	name  string
	inner adk.ChatModelAgentMiddleware
}

func WrapAgentMiddleware(name string, inner adk.ChatModelAgentMiddleware) agentcore.AgentMiddleware {
	return &agentMiddleware{name: name, inner: inner}
}

func (m *agentMiddleware) Name() string {
	if m == nil {
		return ""
	}
	return m.name
}

func (m *agentMiddleware) runnerMiddleware() adk.ChatModelAgentMiddleware {
	if m == nil {
		return nil
	}
	return m.inner
}

type toolMiddleware struct {
	name  string
	inner compose.ToolMiddleware
}

func WrapToolMiddleware(name string, inner compose.ToolMiddleware) agentcore.ToolMiddleware {
	return &toolMiddleware{name: name, inner: inner}
}

func (m *toolMiddleware) Name() string {
	if m == nil {
		return ""
	}
	return m.name
}

func (m *toolMiddleware) runnerMiddleware() compose.ToolMiddleware {
	if m == nil {
		return compose.ToolMiddleware{}
	}
	return m.inner
}

func AdaptAgentMiddlewareForRunner(m agentcore.AgentMiddleware) (adk.ChatModelAgentMiddleware, error) {
	if m == nil {
		return nil, fmt.Errorf("middleware is nil")
	}
	handler, ok := m.(interface {
		runnerMiddleware() adk.ChatModelAgentMiddleware
	})
	if !ok || handler.runnerMiddleware() == nil {
		return nil, fmt.Errorf("unsupported agent middleware: %T", m)
	}
	return handler.runnerMiddleware(), nil
}

func AdaptAgentMiddlewaresForRunner(middlewares []agentcore.AgentMiddleware) ([]adk.ChatModelAgentMiddleware, error) {
	result := make([]adk.ChatModelAgentMiddleware, 0, len(middlewares))
	for _, m := range middlewares {
		if m == nil {
			continue
		}
		handler, err := AdaptAgentMiddlewareForRunner(m)
		if err != nil {
			return nil, err
		}
		result = append(result, handler)
	}
	return result, nil
}

func AdaptToolMiddlewareForRunner(m agentcore.ToolMiddleware) (compose.ToolMiddleware, error) {
	if m == nil {
		return compose.ToolMiddleware{}, fmt.Errorf("middleware is nil")
	}
	handler, ok := m.(interface{ runnerMiddleware() compose.ToolMiddleware })
	if !ok {
		return compose.ToolMiddleware{}, fmt.Errorf("unsupported tool middleware: %T", m)
	}
	return handler.runnerMiddleware(), nil
}

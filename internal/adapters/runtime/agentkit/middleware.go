package agentkit

import (
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

// WrapAgentMiddleware 创建显式绑定到 AgentKit 的扩展句柄。
func WrapAgentMiddleware(name string, inner adk.ChatModelAgentMiddleware) runtimeport.AgentMiddleware {
	return runtimeport.BindAgentMiddleware("agentkit", name, inner)
}

// WrapToolMiddleware 创建显式绑定到 AgentKit 的工具扩展句柄。
func WrapToolMiddleware(name string, inner compose.ToolMiddleware) runtimeport.ToolMiddleware {
	return runtimeport.BindToolMiddleware("agentkit", name, inner)
}

func AdaptAgentMiddlewareForRunner(m runtimeport.AgentMiddleware) (adk.ChatModelAgentMiddleware, error) {
	if m == nil {
		return nil, fmt.Errorf("middleware is nil")
	}
	native, err := m.Native("agentkit")
	if err != nil {
		return nil, err
	}
	handler, ok := native.(adk.ChatModelAgentMiddleware)
	if !ok || handler == nil {
		return nil, fmt.Errorf("invalid native agent middleware: %s", m.Name())
	}
	return handler, nil
}

func AdaptAgentMiddlewaresForRunner(middlewares []runtimeport.AgentMiddleware) ([]adk.ChatModelAgentMiddleware, error) {
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

func AdaptToolMiddlewareForRunner(m runtimeport.ToolMiddleware) (compose.ToolMiddleware, error) {
	if m == nil {
		return compose.ToolMiddleware{}, fmt.Errorf("middleware is nil")
	}
	native, err := m.Native("agentkit")
	if err != nil {
		return compose.ToolMiddleware{}, err
	}
	handler, ok := native.(compose.ToolMiddleware)
	if !ok {
		return compose.ToolMiddleware{}, fmt.Errorf("invalid native tool middleware: %s", m.Name())
	}
	return handler, nil
}

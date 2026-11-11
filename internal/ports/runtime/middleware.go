package runtime

import "fmt"

// AgentMiddleware 是运行时工厂创建的不透明智能体扩展句柄。
// 它只可由同一 backend 消费；跨运行时的业务扩展使用 ports/hooks 契约。
type AgentMiddleware = *agentMiddlewareHandle

// ToolMiddleware 是运行时工厂创建的不透明工具扩展句柄。
type ToolMiddleware = *toolMiddlewareHandle

type middlewareBinding struct {
	name    string
	backend string
	native  any
}
type agentMiddlewareHandle struct{ middlewareBinding }
type toolMiddlewareHandle struct{ middlewareBinding }

// BindAgentMiddleware 将 SDK 中间件绑定到指定运行时，应用层仅持有句柄。
func BindAgentMiddleware(backend, name string, native any) AgentMiddleware {
	return &agentMiddlewareHandle{middlewareBinding{name: name, backend: backend, native: native}}
}

// BindToolMiddleware 将 SDK 工具中间件绑定到指定运行时。
func BindToolMiddleware(backend, name string, native any) ToolMiddleware {
	return &toolMiddlewareHandle{middlewareBinding{name: name, backend: backend, native: native}}
}

// Name 返回扩展名称，仅用于诊断。
func (b middlewareBinding) Name() string { return b.name }

// Native 供对应适配器取回原生扩展；显式拒绝不兼容运行时。
func (b middlewareBinding) Native(backend string) (any, error) {
	if backend == "" || b.backend != backend || b.native == nil {
		return nil, fmt.Errorf("middleware %q is not bound to runtime %q", b.name, backend)
	}
	return b.native, nil
}

// Package execution 定义各入口共享的应用执行依赖及其上下文绑定规则。
package execution

import (
	"context"
	appprocess "fkteams/internal/app/process"
	storageport "fkteams/internal/ports/storage"

	agents "fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/agent/catalog/toolmeta"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
	modelregistry "fkteams/internal/runtime/model"
)

// Dependencies 保存应用实例的执行能力；副本共享注册表，不包含会话状态或取消信号。
type Dependencies struct {
	Processes     *appprocess.Service
	Documents     storageport.Documents
	Runtime       runtimeport.Runtime
	Interrupt     runtimeport.InterruptRuntime
	ModelRegistry *modelregistry.Registry
	ToolRegistry  *apptools.ToolGroupRegistry
	ToolDisplays  *toolmeta.Registry
	AgentRegistry *agents.Registry
	HookBus       *hooks.Bus
}

// Context 将执行能力绑定到当前任务上下文，保留任务自身的值、期限与取消语义。
func (d Dependencies) Context(parent context.Context) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	parent = context.WithValue(parent, documentsKey{}, documentDependency{d.Documents})
	parent = context.WithValue(parent, processesKey{}, d.Processes)
	ctx := runtimeport.WithRuntime(parent, d.Runtime)
	ctx = runtimeport.WithInterruptRuntime(ctx, d.Interrupt)
	ctx = modelregistry.WithRegistry(ctx, d.ModelRegistry)
	ctx = apptools.WithRegistry(ctx, d.ToolRegistry)
	ctx = toolmeta.WithRegistry(ctx, d.ToolDisplays)
	ctx = agents.WithRegistry(ctx, d.AgentRegistry)
	return hooks.WithBus(ctx, d.HookBus)
}

// FromContext 提取执行能力，供后台服务绑定到自己的任务上下文。
func FromContext(ctx context.Context) Dependencies {
	var d Dependencies
	if ctx == nil {
		return d
	}
	if dependency, ok := ctx.Value(documentsKey{}).(documentDependency); ok {
		d.Documents = dependency.documents
	}
	d.Processes, _ = ctx.Value(processesKey{}).(*appprocess.Service)
	d.Runtime, _ = runtimeport.RuntimeFromContext(ctx)
	d.Interrupt, _ = runtimeport.InterruptRuntimeFromContext(ctx)
	d.ModelRegistry, _ = modelregistry.RegistryFromContext(ctx)
	d.ToolRegistry, _ = apptools.RegistryFromContext(ctx)
	d.ToolDisplays, _ = toolmeta.RegistryFromContext(ctx)
	d.AgentRegistry, _ = agents.RegistryFromContext(ctx)
	d.HookBus = hooks.FromContext(ctx)
	return d
}

type documentsKey struct{}
type documentDependency struct{ documents storageport.Documents }

type processesKey struct{}

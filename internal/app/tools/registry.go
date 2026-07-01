package tools

import (
	"context"
	"fmt"
	"sync"

	runtimeport "fkteams/internal/ports/runtime"
	storageport "fkteams/internal/ports/storage"
	toolport "fkteams/internal/ports/tools"
	"fkteams/internal/runtime/resources"
)

type ToolResolveContext struct {
	WorkspaceDir  string
	SessionsDir   string
	RuntimeDir    string
	Cleaner       *resources.Cleaner
	Config        any
	HistoryReader storageport.SessionMessageReader
}

type ToolGroupFactory func(ctx ToolResolveContext) ([]runtimeport.Tool, error)

type ToolGroupRegistration struct {
	Info    ToolGroupInfo
	Factory ToolGroupFactory
}

type ToolGroupRegistry struct {
	mu          sync.RWMutex
	order       []string
	groups      map[string]toolGroupEntry
	frozen      bool
	mcpProvider toolport.MCPProvider
	resolveCtx  ToolResolveContext
}

type toolGroupEntry struct {
	info    ToolGroupInfo
	factory ToolGroupFactory
}

type registryContextKey struct{}

func NewToolGroupRegistry(resolveContext ...ToolResolveContext) *ToolGroupRegistry {
	var ctx ToolResolveContext
	if len(resolveContext) > 0 {
		ctx = resolveContext[0]
	}
	return &ToolGroupRegistry{groups: make(map[string]toolGroupEntry), resolveCtx: ctx}
}

func WithRegistry(ctx context.Context, registry *ToolGroupRegistry) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if registry == nil {
		return ctx
	}
	return context.WithValue(ctx, registryContextKey{}, registry)
}

func RegistryFromContext(ctx context.Context) (*ToolGroupRegistry, bool) {
	if ctx == nil {
		return nil, false
	}
	registry, ok := ctx.Value(registryContextKey{}).(*ToolGroupRegistry)
	return registry, ok && registry != nil
}

func RequireRegistry(ctx context.Context) (*ToolGroupRegistry, error) {
	if registry, ok := RegistryFromContext(ctx); ok {
		return registry, nil
	}
	return nil, fmt.Errorf("tool registry is not configured")
}

func (r *ToolGroupRegistry) Register(reg ToolGroupRegistration) error {
	if r == nil {
		return fmt.Errorf("tool group registry is nil")
	}
	info := cloneToolGroupInfo(reg.Info)
	if info.Name == "" {
		return fmt.Errorf("tool group name is empty")
	}
	if reg.Factory == nil {
		return fmt.Errorf("tool group %s factory is nil", info.Name)
	}
	if info.DisplayName == "" {
		return fmt.Errorf("tool group %s display name is empty", info.Name)
	}
	if info.Description == "" {
		return fmt.Errorf("tool group %s description is empty", info.Name)
	}
	if info.Category == "" {
		return fmt.Errorf("tool group %s category is empty", info.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return fmt.Errorf("tool group registry is frozen")
	}
	if _, exists := r.groups[info.Name]; exists {
		return fmt.Errorf("tool group %s already registered", info.Name)
	}
	r.groups[info.Name] = toolGroupEntry{info: info, factory: reg.Factory}
	r.order = append(r.order, info.Name)
	return nil
}

func (r *ToolGroupRegistry) Resolve(name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("tool group registry is nil")
	}
	r.mu.RLock()
	entry, ok := r.groups[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	resolveCtx := r.ResolveContext(cleaner)
	tools, err := entry.factory(resolveCtx)
	if err != nil {
		return nil, true, err
	}
	return tools, true, nil
}

func (r *ToolGroupRegistry) ResolveContext(cleaner *resources.Cleaner) ToolResolveContext {
	if r == nil {
		return ToolResolveContext{Cleaner: cleaner}
	}
	r.mu.RLock()
	ctx := r.resolveCtx
	r.mu.RUnlock()
	if cleaner != nil {
		ctx.Cleaner = cleaner
	}
	return ctx
}

func (r *ToolGroupRegistry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.order))
	for _, name := range r.order {
		entry, ok := r.groups[name]
		if !ok || entry.info.Hidden {
			continue
		}
		names = append(names, name)
	}
	return names
}

func (r *ToolGroupRegistry) Infos() []ToolGroupInfo {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ToolGroupInfo, 0, len(r.order))
	for _, name := range r.order {
		entry, ok := r.groups[name]
		if !ok || entry.info.Hidden {
			continue
		}
		infos = append(infos, cloneToolGroupInfo(entry.info))
	}
	return infos
}

func (r *ToolGroupRegistry) Freeze() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

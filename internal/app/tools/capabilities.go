package tools

import (
	"context"
	"fmt"

	"fkteams/internal/app/tools/attachment"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
	"fkteams/internal/runtime/toolpolicy"
)

type BuiltinCapability struct {
	Name     string
	Provider func(ctx ToolResolveContext) ([]runtimeport.Tool, error)
}

var builtinCapabilities = []BuiltinCapability{
	{
		Name: "session_attachment",
		Provider: func(ctx ToolResolveContext) ([]runtimeport.Tool, error) {
			return attachment.GetTools(ctx.HistoryReader)
		},
	},
}

func BuiltinCapabilityNames() []string {
	names := make([]string, 0, len(builtinCapabilities))
	for _, capability := range builtinCapabilities {
		names = append(names, capability.Name)
	}
	return names
}

func GetBuiltinCapabilityTools(ctx context.Context) ([]runtimeport.Tool, error) {
	return GetBuiltinCapabilityToolsWithCleaner(ctx, nil)
}

func GetBuiltinCapabilityToolsWithCleaner(ctx context.Context, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	resolveCtx := ToolResolveContext{Cleaner: cleaner}
	if registry, ok := RegistryFromContext(ctx); ok {
		resolveCtx = registry.ResolveContext(cleaner)
	}
	var result []runtimeport.Tool
	for _, capability := range builtinCapabilities {
		if capability.Provider == nil {
			return nil, fmt.Errorf("builtin capability %s provider is nil", capability.Name)
		}
		resolved, err := capability.Provider(resolveCtx)
		if err != nil {
			return nil, fmt.Errorf("init builtin capability %s: %w", capability.Name, err)
		}
		if err := toolpolicy.MarkPolicyRequired(resolved); err != nil {
			return nil, fmt.Errorf("mark builtin capability %s policy: %w", capability.Name, err)
		}
		result = append(result, resolved...)
	}
	return result, nil
}

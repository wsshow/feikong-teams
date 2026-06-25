package tools

import (
	"fkteams/internal/app/tools/attachment"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
	"fmt"
)

type BuiltinCapability struct {
	Name     string
	Provider func(cleaner *resources.Cleaner) ([]runtimeport.Tool, error)
}

var builtinCapabilities = []BuiltinCapability{
	{
		Name: "session_attachment",
		Provider: func(*resources.Cleaner) ([]runtimeport.Tool, error) {
			return attachment.GetTools()
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

func GetBuiltinCapabilityTools() ([]runtimeport.Tool, error) {
	return GetBuiltinCapabilityToolsWithCleaner(nil)
}

func GetBuiltinCapabilityToolsWithCleaner(cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	var result []runtimeport.Tool
	for _, capability := range builtinCapabilities {
		if capability.Provider == nil {
			return nil, fmt.Errorf("builtin capability %s provider is nil", capability.Name)
		}
		resolved, err := capability.Provider(cleaner)
		if err != nil {
			return nil, fmt.Errorf("init builtin capability %s: %w", capability.Name, err)
		}
		if err := MarkPolicyRequired(resolved); err != nil {
			return nil, fmt.Errorf("mark builtin capability %s policy: %w", capability.Name, err)
		}
		result = append(result, resolved...)
	}
	return result, nil
}

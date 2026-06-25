package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"fkteams/internal/app/appdata"
	"fkteams/internal/app/tools/mcp"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

// workspacePath 返回工作区目录路径
func workspacePath() string {
	return appdata.WorkspaceDir()
}

// runtimeDir 返回脚本运行时环境目录
func runtimeDir() string {
	return filepath.Join(appdata.Dir(), "runtime")
}

func GetToolsByName(name string) ([]runtimeport.Tool, error) {
	return GetToolsByNameWithCleaner(name, nil)
}

// GetToolsByNameWithCleaner 按名称返回工具列表，并按需注册进程级清理函数。
func GetToolsByNameWithCleaner(name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	if resolved, ok, err := defaultRegistry.Resolve(name, cleaner); ok || err != nil {
		return resolved, err
	}
	if name, ok := strings.CutPrefix(name, "mcp-"); ok {
		return mcp.GetToolsByName(name)
	}
	return nil, fmt.Errorf("tool %s not found", name)
}

// BuiltinToolNames 返回所有内置工具组名称
func BuiltinToolNames() []string {
	return defaultRegistry.Names()
}

// GetAllToolNames 返回所有可用的工具名列表（内置 + MCP）
func GetAllToolNames() []string {
	names := make([]string, 0, len(BuiltinToolNames()))
	names = append(names, BuiltinToolNames()...)
	mcpGroups, err := mcp.GetAllToolGroups()
	if err == nil {
		for name := range mcpGroups {
			names = append(names, "mcp-"+name)
		}
	}
	return names
}

package tools

import (
	"fkteams/common"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/tools/mcp"
	"fmt"
	"path/filepath"
	"strings"
)

// workspacePath 返回工作区目录路径
func workspacePath() string {
	return common.WorkspaceDir()
}

// runtimeDir 返回脚本运行时环境目录
func runtimeDir() string {
	return filepath.Join(common.AppDir(), "runtime")
}

func GetToolsByName(name string) ([]runtimeport.Tool, error) {
	return GetToolsByNameWithCleaner(name, nil)
}

// GetToolsByNameWithCleaner 按名称返回工具列表，并按需注册进程级清理函数。
func GetToolsByNameWithCleaner(name string, cleaner *common.ResourceCleaner) ([]runtimeport.Tool, error) {
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

package tools

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
)

// ToolGroupInfo 描述可在自定义智能体中配置的工具组。
type ToolGroupInfo struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Builtin       bool     `json:"builtin"`
	IncludedTools []string `json:"included_tools,omitempty"`
	Hidden        bool     `json:"-"`
}

// BuiltinToolInfos 返回内置可配置工具组信息。
func BuiltinToolInfos() []ToolGroupInfo {
	return defaultRegistry.Infos()
}

// GetAllToolInfos 返回所有可配置工具组信息（内置 + MCP）。
func GetAllToolInfos() []ToolGroupInfo {
	infos := BuiltinToolInfos()
	mcpGroups, err := GetAllMCPToolGroups()
	if err != nil {
		return infos
	}
	for name, group := range mcpGroups {
		info := ToolGroupInfo{
			Name:          "mcp-" + name,
			DisplayName:   "MCP: " + name,
			Description:   group.Desc,
			Category:      "MCP",
			Builtin:       false,
			IncludedTools: toolNames(group.Tools),
		}
		if info.Description == "" {
			info.Description = "来自 MCP 服务 " + name + " 的工具组。"
		}
		infos = append(infos, info)
	}
	return infos
}

func toolNames(list []runtimeport.Tool) []string {
	names := make([]string, 0, len(list))
	for _, t := range list {
		info, err := t.Info(context.Background())
		if err == nil && info != nil && info.Name != "" {
			names = append(names, info.Name)
		}
	}
	return names
}

func cloneToolGroupInfo(info ToolGroupInfo) ToolGroupInfo {
	info.IncludedTools = append([]string(nil), info.IncludedTools...)
	return info
}

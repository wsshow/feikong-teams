package mcp

import runtimeport "fkteams/internal/ports/runtime"

type ToolGroup struct {
	Name  string
	Desc  string
	Tools []runtimeport.Tool
}

type DictToolGroup map[string]ToolGroup

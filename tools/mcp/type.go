package mcp

import "github.com/cloudwego/eino/components/tool"

type ToolGroup struct {
	Name  string
	Desc  string
	Tools []tool.BaseTool
}

type DictToolGroup map[string]ToolGroup

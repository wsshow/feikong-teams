package search

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

func GetTools() (tools []tool.BaseTool, err error) {
	duckduckgoTool, err := NewDuckDuckGoTool(context.Background())
	if err != nil {
		return nil, err
	}
	tools = append(tools, duckduckgoTool)
	return tools, nil
}

package search

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
)

func GetTools() (tools []runtimeport.Tool, err error) {
	duckduckgoTool, err := NewDuckDuckGoTool(context.Background())
	if err != nil {
		return nil, err
	}
	tools = append(tools, duckduckgoTool)
	return tools, nil
}

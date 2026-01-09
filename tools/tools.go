package tools

import (
	"context"
	"fkteams/tools/command"
	"fkteams/tools/file"
	"fkteams/tools/mcp"
	"fkteams/tools/search"
	"fkteams/tools/ssh"
	"fkteams/tools/todo"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

func GetToolsByName(name string) ([]tool.BaseTool, error) {
	switch name {
	case "file":
		return file.GetTools()
	case "ssh":
		return ssh.GetTools()
	case "todo":
		return todo.GetTools()
	case "command":
		return command.GetTools()
	case "search":
		duckduckgoTool, err := search.NewDuckDuckGoTool(context.Background())
		return []tool.BaseTool{duckduckgoTool}, err
	default:
		if name, ok := strings.CutPrefix(name, "mcp-"); ok {
			return mcp.GetToolsByName(name)
		}
		return nil, fmt.Errorf("tool %s not found", name)
	}
}

package tools

import (
	"context"
	"fkteams/tools/command"
	"fkteams/tools/file"
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
		if strings.HasPrefix(name, "mcp-") {
			return nil, fmt.Errorf("mcp tools are not supported in this environment")
		}
		return nil, fmt.Errorf("tool %s not found", name)
	}
}

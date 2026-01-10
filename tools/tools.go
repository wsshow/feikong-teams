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
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

func GetToolsByName(name string) ([]tool.BaseTool, error) {
	switch name {
	case "file":
		safeDir := "./code"
		codeDirEnv := os.Getenv("FEIKONG_FILE_TOOL_DIR")
		if codeDirEnv != "" {
			safeDir = codeDirEnv
		}
		fileTools, err := file.NewFileTools(safeDir)
		if err != nil {
			return nil, fmt.Errorf("初始化文件工具失败: %w", err)
		}
		return fileTools.GetTools()
	case "todo":
		todoDir := "./todo"
		todoDirEnv := os.Getenv("FEIKONG_TODO_TOOL_DIR")
		if todoDirEnv != "" {
			todoDir = todoDirEnv
		}
		todoTools, err := todo.NewTodoTools(todoDir)
		if err != nil {
			return nil, fmt.Errorf("初始化Todo工具失败: %w", err)
		}
		return todoTools.GetTools()
	case "ssh":
		host := os.Getenv("FEIKONG_SSH_HOST")
		username := os.Getenv("FEIKONG_SSH_USERNAME")
		password := os.Getenv("FEIKONG_SSH_PASSWORD")
		if host == "" || username == "" || password == "" {
			return nil, fmt.Errorf("SSH 连接信息未配置。请设置以下环境变量：FEIKONG_SSH_HOST, FEIKONG_SSH_USERNAME, FEIKONG_SSH_PASSWORD")
		}
		sshTools, err := ssh.NewSSHTools(host, username, password)
		if err != nil {
			return nil, fmt.Errorf("初始化 SSH 工具失败: %w", err)
		}
		return sshTools.GetTools()
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

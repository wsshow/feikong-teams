package tools

import (
	"context"
	"fkteams/tools/command"
	"fkteams/tools/file"
	"fkteams/tools/mcp"
	"fkteams/tools/search"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

func GetToolsByName(name string) ([]tool.BaseTool, error) {
	switch name {
	case "file":
		safeDir := "./code"
		codeDirEnv := os.Getenv("FEIKONG_FILE_TOOL_SAFE_DIR")
		if codeDirEnv != "" {
			safeDir = codeDirEnv
		}
		fileTools, err := file.NewFileTools(safeDir)
		if err != nil {
			return nil, fmt.Errorf("初始化文件工具失败: %w", err)
		}
		return fileTools.GetTools()
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

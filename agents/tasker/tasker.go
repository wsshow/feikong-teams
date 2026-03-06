package tasker

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	toolSearch "fkteams/tools/search"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/adk"
	etool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent(ctx context.Context) adk.Agent {
	workspaceDir := "./workspace"
	if d := os.Getenv("FEIKONG_WORKSPACE_DIR"); d != "" {
		workspaceDir = d
	}

	// 搜索工具
	duckTool, err := toolSearch.NewDuckDuckGoTool(ctx)
	if err != nil {
		log.Fatal("初始化搜索工具失败:", err)
	}

	// Fetch 工具
	fetchTools, err := fetch.GetTools()
	if err != nil {
		log.Fatal("初始化 Fetch 工具失败:", err)
	}

	// 命令执行工具
	cmdTools, err := command.GetTools()
	if err != nil {
		log.Fatal("初始化命令工具失败:", err)
	}

	// 文件操作工具
	fileToolsInstance, err := file.NewFileTools(workspaceDir)
	if err != nil {
		log.Fatal("初始化文件工具失败:", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建文件工具失败:", err)
	}

	var toolList []etool.BaseTool
	toolList = append(toolList, duckTool)
	toolList = append(toolList, fetchTools...)
	toolList = append(toolList, cmdTools...)
	toolList = append(toolList, fileTools...)

	systemMessages, err := TaskerPromptTemplate.Format(ctx, map[string]any{
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"workspace_dir": workspaceDir,
	})
	if err != nil {
		log.Fatal("格式化提示词失败:", err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "任务官",
		Description:   "后台定时任务专属执行官，独立完成信息检索、数据分析、命令执行等各类定时任务，输出严谨可靠的结果。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries: common.MaxRetries,
		},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
	if err != nil {
		log.Fatal("创建任务官智能体失败:", err)
	}
	return a
}

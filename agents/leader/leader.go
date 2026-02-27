package leader

import (
	"context"
	"fkteams/agents/common"
	"fkteams/agents/leader/skills"
	"fkteams/agents/leader/summary"
	"fkteams/tools/file"
	"fkteams/tools/todo"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

var globalTodoToolsInstance *todo.TodoTools

func NewAgent(ctx context.Context) adk.Agent {

	safeDir := "./workspace"
	todoDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
	if todoDirEnv != "" {
		safeDir = todoDirEnv
	}

	// 创建 Todo 工具实例
	todoToolsInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		log.Fatal("初始化 Todo 工具失败:", err)
	}

	// 保存实例以便后续操作
	globalTodoToolsInstance = todoToolsInstance

	// 创建 Todo 工具
	todoTools, err := todoToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建 Todo 工具失败:", err)
	}

	// 初始化文件工具
	fileToolsInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		log.Fatalf("初始化文件工具失败: %v", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建文件工具失败:", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, todoTools...)
	toolList = append(toolList, fileTools...)

	// 加载技能
	skillsMiddleware, err := skills.New(ctx, safeDir)
	if err != nil {
		log.Fatal(err)
	}

	// 上下文压缩
	summaryMiddleware, err := summary.New(ctx, &summary.Config{
		Model:                      common.NewChatModel(),
		SystemPrompt:               summary.PromptOfSummary,
		MaxTokensBeforeSummary:     80 * 1024,
		MaxTokensForRecentMessages: 25 * 1024,
	})
	if err != nil {
		log.Fatal(err)
	}

	systemMessages, err := LeaderPromptTemplate.Format(ctx, map[string]any{
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"team_members":  ctx.Value("team_members"),
		"workspace_dir": safeDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "统御",
		Description:   "团队管理者，善于规划和分配任务。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		Middlewares:   []adk.AgentMiddleware{skillsMiddleware, summaryMiddleware},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

func ClearTodoTool() error {
	if globalTodoToolsInstance == nil {
		return nil
	}
	// 使用TodoClear方法清空所有待办事项
	_, err := globalTodoToolsInstance.TodoClear(context.Background(), &todo.TodoClearRequest{})
	if err != nil {
		return err
	}
	return nil
}

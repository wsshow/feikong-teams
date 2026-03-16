package assistant

import (
	"context"
	"fkteams/agents/common"
	"fkteams/agents/middlewares/skills"
	"fkteams/agents/middlewares/summary"
	toolwarperror "fkteams/agents/middlewares/tools/warperror"
	"fkteams/tools/command"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	"fkteams/tools/scheduler"
	"fkteams/tools/search"
	"fkteams/tools/todo"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()

	safeDir := "./workspace"
	if dir := os.Getenv("FEIKONG_WORKSPACE_DIR"); dir != "" {
		safeDir = dir
	}

	var toolList []tool.BaseTool

	// 核心工具：带审批功能的命令行
	smartTools, err := command.NewCommandTools(safeDir).GetTools()
	if err != nil {
		log.Fatal("初始化命令行工具失败:", err)
	}
	toolList = append(toolList, smartTools...)

	// 文件工具
	fileInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		log.Fatalf("初始化文件工具失败: %v", err)
	}
	fileTools, err := fileInstance.GetTools()
	if err != nil {
		log.Fatal("创建文件工具失败:", err)
	}
	toolList = append(toolList, fileTools...)

	// 待办事项工具
	todoInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		log.Fatal("初始化 Todo 工具失败:", err)
	}
	todoTools, err := todoInstance.GetTools()
	if err != nil {
		log.Fatal("创建 Todo 工具失败:", err)
	}
	toolList = append(toolList, todoTools...)

	// 定时任务工具
	schedulerInstance, err := scheduler.InitGlobal(safeDir)
	if err != nil {
		log.Fatalf("初始化定时任务调度器失败: %v", err)
	}
	schedulerTools, err := schedulerInstance.GetTools()
	if err != nil {
		log.Fatal("创建定时任务工具失败:", err)
	}
	toolList = append(toolList, schedulerTools...)

	// 搜索工具
	searchTool, err := search.NewDuckDuckGoTool(ctx)
	if err != nil {
		log.Fatal("初始化搜索工具失败:", err)
	}
	toolList = append(toolList, searchTool)

	// 网页抓取工具
	fetchTools, err := fetch.GetTools()
	if err != nil {
		log.Fatal("初始化 fetch 工具失败:", err)
	}
	toolList = append(toolList, fetchTools...)

	// 技能中间件
	skillsMiddleware, err := skills.New(ctx, safeDir)
	if err != nil {
		log.Fatal(err)
	}

	// 上下文压缩中间件
	summaryMiddleware, err := summary.New(ctx, &summary.Config{
		Model:                      common.NewChatModel(),
		SystemPrompt:               summary.PromptOfSummary,
		MaxTokensBeforeSummary:     80 * 1024,
		MaxTokensForRecentMessages: 25 * 1024,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 工具错误处理中间件
	warperrorMiddleware := toolwarperror.NewAgentMiddleware(nil)

	systemMessages, err := AssistantPromptTemplate.Format(ctx, map[string]any{
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"os_type":       runtime.GOOS,
		"os_arch":       runtime.GOARCH,
		"workspace_dir": safeDir,
	})
	if err != nil {
		log.Fatal(err)
	}

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小助",
		Description:   "个人全能助手，通过命令执行工具和文件操作完成各种任务，危险操作需要用户审批。",
		Instruction:   systemMessages[0].Content,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
		Middlewares: []adk.AgentMiddleware{
			warperrorMiddleware,
			summaryMiddleware,
		},
		Handlers: []adk.ChatModelAgentMiddleware{skillsMiddleware},
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

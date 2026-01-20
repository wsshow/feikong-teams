package leader

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/todo"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

var globalTodoToolsInstance *todo.TodoTools

func NewAgent(ctx context.Context) adk.Agent {

	todoDir := "./todo"
	todoDirEnv := os.Getenv("FEIKONG_TODO_TOOL_DIR")
	if todoDirEnv != "" {
		todoDir = todoDirEnv
	}

	// 创建 Todo 工具实例
	todoToolsInstance, err := todo.NewTodoTools(todoDir)
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

	systemMessages, err := LeaderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"team_members": ctx.Value("team_members"),
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
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: todoTools,
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

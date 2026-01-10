package leader

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/todo"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

var globalTodoToolsInstance *todo.TodoTools

func NewAgent() adk.Agent {
	ctx := context.Background()

	// 初始化 Todo 工具
	// 获取可执行文件的路径
	execPath, err := os.Executable()
	if err != nil {
		log.Fatal("无法获取可执行文件路径:", err)
	}

	// 获取可执行文件所在的目录
	execDir := filepath.Dir(execPath)

	// 设置 todo 目录为可执行文件同级的目录
	todoDir := filepath.Join(execDir, "todo")

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

package leader

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/todo"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

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

	// 初始化 Todo 工具
	if err := todo.InitTodoTool(todoDir); err != nil {
		log.Fatal("初始化 Todo 工具失败:", err)
	}

	fmt.Printf("[tips] Todo 工具已初始化，存储路径: %s\n", todoDir)

	// 创建 Todo 工具
	todoTools, err := todo.GetTools()
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

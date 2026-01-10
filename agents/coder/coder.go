package coder

import (
	"context"
	"fkteams/agents/common"
	toolFile "fkteams/tools/file"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()

	codeDir := "./code"
	codeDirEnv := os.Getenv("FEIKONG_FILE_TOOL_DIR")
	if codeDirEnv != "" {
		codeDir = codeDirEnv
	}

	// 创建文件工具实例
	fileToolsInstance, err := toolFile.NewFileTools(codeDir)
	if err != nil {
		log.Fatal("初始化文件工具失败:", err)
	}

	fmt.Printf("[tips] 文件工具已限制在目录: %s\n", codeDir)

	// 创建文件操作工具
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		log.Fatal(err)
	}

	// 格式化系统消息
	systemMessages, err := CoderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	// 创建智能体
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小码",
		Description:   "代码专家，擅长读写和处理代码文件，能够帮助用户完成各种编程任务。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: fileTools,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

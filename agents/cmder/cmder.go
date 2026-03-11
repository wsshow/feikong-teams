package cmder

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/script/uv"
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

	workspaceDir := "./workspace"
	workspaceDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
	if workspaceDirEnv != "" {
		workspaceDir = workspaceDirEnv
	}

	// 创建 CLI 操作工具
	cliTools, err := command.GetTools()
	if err != nil {
		log.Fatal("创建 CLI 工具失败:", err)
	}

	// 初始化 uv 工具
	uvToolsInstance, err := uv.NewUVTools(workspaceDir)
	if err != nil {
		log.Fatal("初始化uv工具失败:", err)
	}
	uvTools, err := uvToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建uv工具失败:", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, cliTools...)
	toolList = append(toolList, uvTools...)

	// 格式化系统消息
	systemMessages, err := CmderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"os_type":      runtime.GOOS,
		"os_arch":      runtime.GOARCH,
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	// 创建智能体
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小令",
		Description:   "命令行专家，擅长通过命令行操作完成任务，能够根据操作系统环境执行合适的命令。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
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

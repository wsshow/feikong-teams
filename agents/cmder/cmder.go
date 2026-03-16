package cmder

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/script/uv"
	"fmt"
	"runtime"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()

	workspaceDir := common.WorkspaceDir()

	cmdTools, err := command.NewCommandTools(workspaceDir).GetTools()
	if err != nil {
		return nil, fmt.Errorf("init command tools: %w", err)
	}

	uvToolsInstance, err := uv.NewUVTools(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("init uv tools: %w", err)
	}
	uvTools, err := uvToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create uv tools: %w", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, cmdTools...)
	toolList = append(toolList, uvTools...)

	systemMessages, err := CmderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"os_type":      runtime.GOOS,
		"os_arch":      runtime.GOARCH,
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小令",
		Description:   "命令行专家，擅长通过命令行操作完成任务，能够根据操作系统环境执行合适的命令。",
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
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
}

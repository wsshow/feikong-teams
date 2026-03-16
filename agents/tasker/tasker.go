package tasker

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	toolSearch "fkteams/tools/search"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
	etool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	workspaceDir := common.WorkspaceDir()

	duckTool, err := toolSearch.NewDuckDuckGoTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("init search tool: %w", err)
	}

	fetchTools, err := fetch.GetTools()
	if err != nil {
		return nil, fmt.Errorf("init fetch tools: %w", err)
	}

	cmdTools, err := command.NewCommandTools(workspaceDir, command.WithApprovalMode(command.ApprovalModeReject)).GetTools()
	if err != nil {
		return nil, fmt.Errorf("init command tools: %w", err)
	}

	fileToolsInstance, err := file.NewFileTools(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
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
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "任务官",
		Description:   "后台定时任务专属执行官，独立完成信息检索、数据分析、命令执行等各类定时任务，输出严谨可靠的结果。",
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

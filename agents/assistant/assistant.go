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
	"fmt"
	"runtime"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()

	safeDir := common.WorkspaceDir()

	var toolList []tool.BaseTool

	smartTools, err := command.NewCommandTools(safeDir).GetTools()
	if err != nil {
		return nil, fmt.Errorf("init command tools: %w", err)
	}
	toolList = append(toolList, smartTools...)

	fileInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}
	toolList = append(toolList, fileTools...)

	todoInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init todo tools: %w", err)
	}
	todoTools, err := todoInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create todo tools: %w", err)
	}
	toolList = append(toolList, todoTools...)

	schedulerInstance, err := scheduler.InitGlobal(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init scheduler: %w", err)
	}
	schedulerTools, err := schedulerInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create scheduler tools: %w", err)
	}
	toolList = append(toolList, schedulerTools...)

	searchTool, err := search.NewDuckDuckGoTool(ctx)
	if err != nil {
		return nil, fmt.Errorf("init search tool: %w", err)
	}
	toolList = append(toolList, searchTool)

	fetchTools, err := fetch.GetTools()
	if err != nil {
		return nil, fmt.Errorf("init fetch tools: %w", err)
	}
	toolList = append(toolList, fetchTools...)

	skillsMiddleware, err := skills.New(ctx, safeDir)
	if err != nil {
		return nil, fmt.Errorf("init skills middleware: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	summaryMiddleware, err := summary.New(ctx, &summary.Config{
		Model:                      chatModel,
		SystemPrompt:               summary.PromptOfSummary,
		MaxTokensBeforeSummary:     80 * 1024,
		MaxTokensForRecentMessages: 25 * 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("init summary middleware: %w", err)
	}

	warperrorMiddleware := toolwarperror.NewAgentMiddleware(nil)

	systemMessages, err := AssistantPromptTemplate.Format(ctx, map[string]any{
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"os_type":       runtime.GOOS,
		"os_arch":       runtime.GOARCH,
		"workspace_dir": safeDir,
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小助",
		Description:   "个人全能助手，通过命令执行工具和文件操作完成各种任务，危险操作需要用户审批。",
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
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
}

package leader

import (
	"context"
	"fkteams/agents/common"
	"fkteams/agents/middlewares/skills"
	"fkteams/agents/middlewares/summary"
	"fkteams/agents/middlewares/tools/warperror"
	"fkteams/tools/file"
	"fkteams/tools/scheduler"
	"fkteams/tools/todo"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

var globalTodoToolsInstance *todo.TodoTools

func NewAgent(ctx context.Context) (adk.Agent, error) {

	safeDir := common.WorkspaceDir()

	todoToolsInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init todo tools: %w", err)
	}
	globalTodoToolsInstance = todoToolsInstance

	todoTools, err := todoToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create todo tools: %w", err)
	}

	fileToolsInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	schedulerInstance, err := scheduler.InitGlobal(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init scheduler: %w", err)
	}
	schedulerTools, err := schedulerInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create scheduler tools: %w", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, todoTools...)
	toolList = append(toolList, fileTools...)
	toolList = append(toolList, schedulerTools...)

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

	warperrorMiddleware := warperror.NewAgentMiddleware(nil)

	systemMessages, err := LeaderPromptTemplate.Format(ctx, map[string]any{
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"team_members":  ctx.Value("team_members"),
		"workspace_dir": safeDir,
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "统御",
		Description:   "团队管理者，善于规划和分配任务。",
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

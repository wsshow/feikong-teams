package analyst

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/doc"
	"fkteams/tools/excel"
	"fkteams/tools/file"
	"fkteams/tools/script/uv"
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

	todoToolsInstance, err := todo.NewTodoTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init todo tools: %w", err)
	}
	todoTools, err := todoToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create todo tools: %w", err)
	}

	excelToolsInstance, err := excel.NewExcelTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init excel tools: %w", err)
	}
	excelTools, err := excelToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create excel tools: %w", err)
	}

	fileToolsInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	uvToolsInstance, err := uv.NewUVTools(safeDir)
	if err != nil {
		return nil, fmt.Errorf("init uv tools: %w", err)
	}
	uvTools, err := uvToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create uv tools: %w", err)
	}

	docTools, err := doc.GetTools()
	if err != nil {
		return nil, fmt.Errorf("init doc tools: %w", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, todoTools...)
	toolList = append(toolList, excelTools...)
	toolList = append(toolList, fileTools...)
	toolList = append(toolList, uvTools...)
	toolList = append(toolList, docTools...)

	systemMessages, err := AnalystPromptTemplate.Format(ctx, map[string]any{
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"os":            runtime.GOOS,
		"workspace_dir": safeDir,
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小析",
		Description:   "数据分析专家，擅长使用 Excel、Python 脚本和文档处理工具，从复杂数据和文档中提取有价值的信息并提供专业洞察。",
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

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

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
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

	return common.NewAgentBuilder("小析", "数据分析专家，擅长使用 Excel、Python 脚本和文档处理工具，从复杂数据和文档中提取有价值的信息并提供专业洞察。").
		WithTemplate(AnalystPromptTemplate).
		WithTemplateVar("os", runtime.GOOS).
		WithTemplateVar("workspace_dir", safeDir).
		WithTools(todoTools...).
		WithTools(excelTools...).
		WithTools(fileTools...).
		WithTools(uvTools...).
		WithTools(docTools...).
		Build(context.Background())
}

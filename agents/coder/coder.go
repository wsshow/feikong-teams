package coder

import (
	"context"
	"fkteams/agents/common"
	toolFile "fkteams/tools/file"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	codeDir := common.WorkspaceDir()

	fileToolsInstance, err := toolFile.NewFileTools(codeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	return common.NewAgentBuilder("小码", "代码专家，擅长读写和处理代码文件，能够帮助用户完成各种编程任务。").
		WithTemplate(CoderPromptTemplate).
		WithTemplateVar("code_dir", codeDir).
		WithTools(fileTools...).
		Build(context.Background())
}

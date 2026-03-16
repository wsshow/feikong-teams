package coder

import (
	"context"
	"fkteams/agents/common"
	toolFile "fkteams/tools/file"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()

	codeDir := common.WorkspaceDir()

	fileToolsInstance, err := toolFile.NewFileTools(codeDir)
	if err != nil {
		return nil, fmt.Errorf("init file tools: %w", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create file tools: %w", err)
	}

	systemMessages, err := CoderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"code_dir":     codeDir,
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小码",
		Description:   "代码专家，擅长读写和处理代码文件，能够帮助用户完成各种编程任务。",
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: fileTools,
			},
		},
	})
}

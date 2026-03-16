package cmder

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/command"
	"fkteams/tools/script/uv"
	"fmt"
	"runtime"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
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

	return common.NewAgentBuilder("小令", "命令行专家，擅长通过命令行操作完成任务，能够根据操作系统环境执行合适的命令。").
		WithTemplate(CmderPromptTemplate).
		WithTemplateVar("os_type", runtime.GOOS).
		WithTemplateVar("os_arch", runtime.GOARCH).
		WithTools(cmdTools...).
		WithTools(uvTools...).
		Build(context.Background())
}

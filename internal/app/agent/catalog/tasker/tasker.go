package tasker

import (
	"context"
	"fkteams/internal/app/agent/catalog/common"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	return common.BuildAgent(ctx, common.Definition{
		Name:        "tasker",
		Description: "后台任务执行器，独立完成定时任务中的检索、分析、命令和文件操作。",
		Instruction: taskerPrompt,
		Profile:     common.ProfileBare,
		TemplateVars: map[string]any{
			"workspace_dir": common.WorkspaceDir(),
		},
		ToolNames: []string{"command_reject", "search", "fetch", "file"},
	})
}

package tools

import (
	"fmt"
	"sync"

	eventlog "fkteams/internal/adapters/storage/file/history"
	gittool "fkteams/internal/adapters/tools/builtin/git"
	schedulertool "fkteams/internal/adapters/tools/builtin/scheduler"
	mcpadapter "fkteams/internal/adapters/tools/mcp"
	"fkteams/internal/app/appdata"
	apptools "fkteams/internal/app/tools"
	"fkteams/internal/app/tools/attachment"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

var (
	registerOnce sync.Once
	registerErr  error
)

func init() {
	if err := RegisterDefaults(); err != nil {
		panic(err)
	}
}

// RegisterDefaults 将工具适配器连接到应用工具注册表。
func RegisterDefaults() error {
	registerOnce.Do(func() {
		attachment.SetSessionMessageReader(eventlog.NewSessionMessageReader(appdata.SessionsDir(), eventlog.GlobalSessionManager))
		apptools.RegisterMCPProvider(mcpadapter.DefaultProvider())
		if err := apptools.RegisterToolGroup(apptools.ToolGroupRegistration{
			Info: apptools.ToolGroupInfo{
				Name:          "git",
				DisplayName:   "Git",
				Description:   "查看状态、提交、分支、日志和差异，适合版本管理和代码变更检查。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"git_status", "git_diff", "git_log", "git_commit"},
			},
			Factory: func(*resources.Cleaner) ([]runtimeport.Tool, error) {
				gitTools, err := gittool.NewGitTools(appdata.WorkspaceDir())
				if err != nil {
					return nil, fmt.Errorf("初始化Git工具失败: %w", err)
				}
				return gitTools.GetTools()
			},
		}); err != nil {
			registerErr = err
			return
		}
		registerErr = apptools.RegisterToolGroup(apptools.ToolGroupRegistration{
			Info: apptools.ToolGroupInfo{
				Name:          "scheduler",
				DisplayName:   "定时任务",
				Description:   "创建、查看和管理自然语言定时任务，适合提醒、周期执行和后台任务。",
				Category:      "自动化",
				Builtin:       true,
				IncludedTools: []string{"schedule_add", "schedule_list", "schedule_cancel", "schedule_delete"},
			},
			Factory: func(*resources.Cleaner) ([]runtimeport.Tool, error) {
				return schedulertool.NewTools(nil).GetTools()
			},
		})
	})
	return registerErr
}

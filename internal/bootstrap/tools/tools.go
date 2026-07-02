package tools

import (
	"fmt"
	"path/filepath"

	eventlog "fkteams/internal/adapters/storage/file/history"
	commandtool "fkteams/internal/adapters/tools/builtin/command"
	doctool "fkteams/internal/adapters/tools/builtin/doc"
	exceltool "fkteams/internal/adapters/tools/builtin/excel"
	fetchtool "fkteams/internal/adapters/tools/builtin/fetch"
	filetool "fkteams/internal/adapters/tools/builtin/file"
	gittool "fkteams/internal/adapters/tools/builtin/git"
	schedulertool "fkteams/internal/adapters/tools/builtin/scheduler"
	buntool "fkteams/internal/adapters/tools/builtin/script/bun"
	uvtool "fkteams/internal/adapters/tools/builtin/script/uv"
	searchtool "fkteams/internal/adapters/tools/builtin/search"
	sshtool "fkteams/internal/adapters/tools/builtin/ssh"
	todotool "fkteams/internal/adapters/tools/builtin/todo"
	mcpadapter "fkteams/internal/adapters/tools/mcp"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/config"
	apptools "fkteams/internal/app/tools"
	"fkteams/internal/app/tools/ask"
	runtimeport "fkteams/internal/ports/runtime"
	toolport "fkteams/internal/ports/tools"
)

func runtimeDir() string {
	return filepath.Join(appdata.Dir(), "runtime")
}

// RegisterDefaults 将工具适配器连接到新的应用工具注册表实例。
func RegisterDefaults(mcpProvider toolport.MCPProvider) (*apptools.ToolGroupRegistry, error) {
	cfg := config.Get()
	sessionsDir := appdata.SessionsDir()
	resolveCtx := apptools.ToolResolveContext{
		WorkspaceDir:  appdata.WorkspaceDir(),
		SessionsDir:   sessionsDir,
		RuntimeDir:    runtimeDir(),
		Config:        cfg,
		HistoryReader: eventlog.NewSessionMessageReader(sessionsDir, eventlog.NewSessionHistoryManager()),
	}
	registry := apptools.NewToolGroupRegistry(resolveCtx)
	if mcpProvider == nil {
		mcpProvider = mcpadapter.NewProvider()
	}
	registry.RegisterMCPProvider(mcpProvider)

	registrations := []apptools.ToolGroupRegistration{
		{
			Info: apptools.ToolGroupInfo{
				Name:          "file",
				DisplayName:   "文件",
				Description:   "读取、搜索、创建和修改工作区文件，适合代码编辑、项目检查和文档整理。",
				Category:      "文件",
				Builtin:       true,
				IncludedTools: []string{"file_read", "file_write", "file_search", "file_list"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				fileTools, err := filetool.NewFileTools(ctx.WorkspaceDir)
				if err != nil {
					return nil, fmt.Errorf("初始化文件工具失败: %w", err)
				}
				return fileTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "todo",
				DisplayName:   "待办事项",
				Description:   "管理任务清单和执行进度，适合长任务拆解、跟踪和复盘。",
				Category:      "协作",
				Builtin:       true,
				IncludedTools: []string{"todo_add", "todo_update", "todo_list"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				todoTools, err := todotool.NewTodoTools(ctx.SessionsDir)
				if err != nil {
					return nil, fmt.Errorf("初始化Todo工具失败: %w", err)
				}
				return todoTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "ask",
				DisplayName:   "向用户提问",
				Description:   "允许智能体在信息不足或需要选择时向用户提问，并等待用户回答后继续。",
				Category:      "协作",
				Builtin:       true,
				IncludedTools: []string{"ask_questions"},
			},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return ask.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "command",
				DisplayName:   "命令执行",
				Description:   "在工作区内执行 shell 命令，适合构建、测试、检查和自动化脚本。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"execute"},
			},
			Factory: commandToolGroup(commandtool.ApprovalModeHITL),
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "command_reject",
				DisplayName:   "命令执行（自动拒绝危险操作）",
				Description:   "后台任务内部使用的命令执行工具，遇到高风险命令时自动拒绝。",
				Category:      "内部",
				Builtin:       true,
				IncludedTools: []string{"execute"},
				Hidden:        true,
			},
			Factory: commandToolGroup(commandtool.ApprovalModeReject),
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "uv",
				DisplayName:   "Python",
				Description:   "使用 uv 创建隔离 Python 环境、安装依赖并执行 Python 代码。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"uv_python"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				uvTools, err := uvtool.NewUVTools(ctx.RuntimeDir, ctx.WorkspaceDir)
				if err != nil {
					return nil, fmt.Errorf("初始化 uv 工具失败: %w", err)
				}
				return uvTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "bun",
				DisplayName:   "JavaScript",
				Description:   "使用 bun 创建隔离 JavaScript 环境、安装依赖并执行 JS/TS 代码。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"bun_javascript"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				bunTools, err := buntool.NewBunTools(ctx.RuntimeDir, ctx.WorkspaceDir)
				if err != nil {
					return nil, fmt.Errorf("初始化 bun 工具失败: %w", err)
				}
				return bunTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "excel",
				DisplayName:   "Excel",
				Description:   "创建、读取和编辑 Excel 工作簿，处理表格数据、公式、样式和工作表。",
				Category:      "数据",
				Builtin:       true,
				IncludedTools: []string{"excel_create", "excel_read", "excel_write", "excel_style"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				excelTools, err := exceltool.NewExcelTools(ctx.WorkspaceDir)
				if err != nil {
					return nil, fmt.Errorf("初始化Excel工具失败: %w", err)
				}
				return excelTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "search",
				DisplayName:   "网络搜索",
				Description:   "检索互联网信息，适合需要时效性、外部资料或交叉验证的问题。",
				Category:      "研究",
				Builtin:       true,
				IncludedTools: []string{"search"},
			},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return searchtool.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "fetch",
				DisplayName:   "网页抓取",
				Description:   "读取指定 URL 的网页内容，适合打开搜索结果、文档页面和公开资料。",
				Category:      "研究",
				Builtin:       true,
				IncludedTools: []string{"fetch"},
			},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return fetchtool.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "doc",
				DisplayName:   "文档",
				Description:   "读取和分析文档文件，支持文档信息、智能读取、按页和按行读取。",
				Category:      "文档",
				Builtin:       true,
				IncludedTools: []string{"doc_info", "doc_smart_read", "doc_read_pages", "doc_read_lines"},
			},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return doctool.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "git",
				DisplayName:   "Git",
				Description:   "查看状态、提交、分支、日志和差异，适合版本管理和代码变更检查。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"git_status", "git_diff", "git_log", "git_commit"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				gitTools, err := gittool.NewGitTools(ctx.WorkspaceDir)
				if err != nil {
					return nil, fmt.Errorf("初始化Git工具失败: %w", err)
				}
				return gitTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "ssh",
				DisplayName:   "SSH",
				Description:   "连接远程服务器执行命令、上传下载文件和查看目录，需要先配置 SSH 连接信息。",
				Category:      "运维",
				Builtin:       true,
				IncludedTools: []string{"ssh_execute", "ssh_upload", "ssh_download", "ssh_list_dir"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				sshCfg := ctx.SSH
				if sshCfg == nil {
					return nil, fmt.Errorf("SSH 连接信息未配置，请在当前智能体的 ssh 配置中设置 host, username, password")
				}
				host := sshCfg.Host
				username := sshCfg.Username
				password := sshCfg.Password
				if host == "" || username == "" || password == "" {
					return nil, fmt.Errorf("SSH 连接信息未配置，请在当前智能体的 ssh 配置中设置 host, username, password")
				}
				sshTools, err := sshtool.NewSSHTools(host, username, password)
				if err != nil {
					return nil, fmt.Errorf("初始化 SSH 工具失败: %w", err)
				}
				if ctx.Cleaner != nil {
					ctx.Cleaner.Add(func() error {
						sshTools.Close()
						return nil
					})
				}
				return sshTools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "scheduler",
				DisplayName:   "定时任务",
				Description:   "创建、查看和管理自然语言定时任务，适合提醒、周期执行和后台任务。",
				Category:      "自动化",
				Builtin:       true,
				IncludedTools: []string{"schedule_add", "schedule_list", "schedule_cancel", "schedule_delete"},
			},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return schedulertool.NewTools(nil).GetTools()
			},
		},
	}

	for _, reg := range registrations {
		if err := registry.Register(reg); err != nil {
			return nil, err
		}
	}
	registry.Freeze()
	return registry, nil
}

func commandToolGroup(mode commandtool.ApprovalMode) apptools.ToolGroupFactory {
	return func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
		if ctx.Cleaner != nil {
			ctx.Cleaner.Add(func() error {
				commandtool.TerminateAll()
				commandtool.CleanupTempFiles(ctx.WorkspaceDir)
				return nil
			})
		}
		return commandtool.NewCommandTools(ctx.WorkspaceDir, commandtool.WithApprovalMode(mode)).GetTools()
	}
}

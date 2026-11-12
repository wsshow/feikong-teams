package tools

import (
	"fmt"
	"path/filepath"
	"sync"

	eventlog "fkteams/internal/adapters/storage/file/history"
	taskstore "fkteams/internal/adapters/storage/task"
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
	javascripttool "fkteams/internal/adapters/tools/javascript"
	mcpadapter "fkteams/internal/adapters/tools/mcp"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/config"
	appsession "fkteams/internal/app/session"
	apptask "fkteams/internal/app/task"
	apptools "fkteams/internal/app/tools"
	"fkteams/internal/app/tools/ask"
	assistanttools "fkteams/internal/app/tools/assistant"
	"fkteams/internal/app/tools/conversations"
	memorytools "fkteams/internal/app/tools/memory"
	tasktools "fkteams/internal/app/tools/tasks"
	runtimeport "fkteams/internal/ports/runtime"
	storageport "fkteams/internal/ports/storage"
	toolport "fkteams/internal/ports/tools"
	"fkteams/internal/runtime/resources"
)

func runtimeDir() string {
	return filepath.Join(appdata.Dir(), "runtime")
}

func appdataJavaScriptStateDir() string {
	return filepath.Join(appdata.JavaScriptDir(), "state")
}

// RegisterDefaults 将工具适配器连接到新的应用工具注册表实例。
func RegisterDefaults(mcpProvider toolport.MCPProvider, stores ...storageport.Documents) (*apptools.ToolGroupRegistry, error) {
	sessionsDir := appdata.SessionsDir()
	sharedLocks := resources.NewKeyedLocks()
	commands := &commandManagers{byCleaner: make(map[*resources.Cleaner]*commandtool.Manager)}
	var documents storageport.Documents
	if len(stores) > 0 {
		documents = stores[0]
	}
	histories := eventlog.NewSessionHistoryManager()
	histories.Documents = documents
	resolveCtx := apptools.ToolResolveContext{
		WorkspaceDir:  appdata.WorkspaceDir(),
		SessionsDir:   sessionsDir,
		RuntimeDir:    runtimeDir(),
		HistoryReader: eventlog.NewSessionMessageReader(sessionsDir, histories),
	}
	registry := apptools.NewToolGroupRegistry(resolveCtx)
	javascriptHost, err := newJavaScriptHost(registry, documents)
	if err != nil {
		return nil, fmt.Errorf("initialize javascript host: %w", err)
	}
	if mcpProvider == nil {
		mcpProvider = mcpadapter.NewProvider()
	}
	registry.RegisterMCPProvider(mcpProvider)

	taskService := apptask.NewService(taskstore.NewStore(documents, filepath.Join(appdata.Dir(), "assistant", "tasks.json")))
	registrations := []apptools.ToolGroupRegistration{
		{
			Info:    apptools.ToolGroupInfo{Name: "tasks", DisplayName: "持续任务", Description: "保存用户目标、进展、下一步和交付结果，支持跨回合续办。", Category: "协作", Builtin: true, IncludedTools: []string{"task_list", "task_save"}},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) { return tasktools.GetTools(taskService) },
		},
		{
			Info:    apptools.ToolGroupInfo{Name: "memory", DisplayName: "长期记忆", Description: "查询、保存、纠正和忘记用户明确指定的长期偏好与背景。", Category: "协作", Builtin: true, IncludedTools: []string{"memory_recall", "memory_save", "memory_forget"}},
			Factory: func(apptools.ToolResolveContext) ([]runtimeport.Tool, error) { return memorytools.GetTools() },
		},
		{
			Info: apptools.ToolGroupInfo{
				Name: "assistant_identity", DisplayName: "助手身份", Description: "查询和修改常驻助手名称。",
				Category: "协作", Builtin: true, Hidden: true, IncludedTools: []string{"assistant_profile", "assistant_rename"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return assistanttools.GetTools(appsession.NewService(eventlog.NewSessionRepository(ctx.SessionsDir, documents)))
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name: "conversations", DisplayName: "会话检索", Description: "列出、搜索和分页读取当前用户的会话历史。",
				Category: "协作", Builtin: true, IncludedTools: []string{"session_search", "session_read"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				return conversations.GetTools(appsession.NewQueryService(eventlog.NewSessionRepository(ctx.SessionsDir, documents), ctx.HistoryReader))
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "file",
				DisplayName:   "文件",
				Description:   "读取、搜索、创建和修改工作区文件，适合代码编辑、项目检查和文档整理。",
				Category:      "文件",
				Builtin:       true,
				IncludedTools: []string{"file_read", "file_write", "file_append", "file_edit", "grep", "file_list", "glob", "file_patch"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				fileTools, err := filetool.NewFileTools(ctx.WorkspaceDir, sharedLocks)
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
				IncludedTools: []string{"todo_add", "todo_list", "todo_update", "todo_delete", "todo_batch_add", "todo_batch_delete", "todo_clear"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				todoTools, err := todotool.NewTodoToolsWithLocks(ctx.SessionsDir, sharedLocks, documents)
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
			Factory: commandToolGroup(commandtool.ApprovalModeHITL, commands),
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
			Factory: commandToolGroup(commandtool.ApprovalModeReject, commands),
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "uv",
				DisplayName:   "Python",
				Description:   "使用 uv 创建隔离 Python 环境、安装依赖并执行 Python 代码。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"uv_init_env", "uv_install_package", "uv_remove_package", "uv_list_package", "uv_clean_env", "uv_run_script", "uv_get_env_info", "uv_run_code", "uv_check_syntax", "uv_format_code"},
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
				IncludedTools: []string{"bun_init_env", "bun_install_package", "bun_remove_package", "bun_list_package", "bun_clean_env", "bun_run_script", "bun_get_env_info"},
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
				IncludedTools: []string{"excel_create_workbook", "excel_open_workbook", "excel_save_workbook", "excel_set_workbook_props", "excel_create_sheet", "excel_delete_sheet", "excel_rename_sheet", "excel_copy_sheet", "excel_set_active_sheet", "excel_set_sheet_visible", "excel_set_cell_value", "excel_get_cell_value", "excel_set_cell_formula", "excel_merge_cells", "excel_unmerge_cells", "excel_set_cell_style", "excel_get_rows", "excel_get_row", "excel_get_col", "excel_get_sheet_data", "excel_get_all_sheets_data", "excel_set_row_height", "excel_set_col_width", "excel_insert_row", "excel_remove_row", "excel_insert_col", "excel_remove_col", "excel_create_style", "excel_set_conditional_format", "excel_add_picture", "excel_add_chart", "excel_batch_create_sheets", "excel_batch_delete_sheets", "excel_batch_set_cell_values", "excel_batch_fill_rows", "excel_batch_fill_cols", "excel_batch_insert_rows", "excel_batch_remove_rows", "excel_batch_insert_cols", "excel_batch_remove_cols", "excel_batch_set_cell_styles"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				excelTools, err := exceltool.NewExcelTools(ctx.WorkspaceDir, sharedLocks)
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
				IncludedTools: []string{"get_document_info", "read_document_smart", "read_document_by_pages", "read_document_by_lines"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				tools, err := doctool.NewTools(ctx.WorkspaceDir)
				if err != nil {
					return nil, err
				}
				return tools.GetTools()
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "git",
				DisplayName:   "Git",
				Description:   "查看状态、提交、分支、日志和差异，适合版本管理和代码变更检查。",
				Category:      "开发",
				Builtin:       true,
				IncludedTools: []string{"git_init", "git_status", "git_add", "git_commit", "git_log", "git_branch", "git_checkout", "git_reset", "git_tag", "git_diff", "git_clean", "git_remove", "git_config", "git_remote"},
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				gitTools, err := gittool.NewGitTools(ctx.WorkspaceDir, sharedLocks)
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
				sshTools, err := sshtool.NewSSHTools(
					host,
					username,
					password,
					sshtool.WithKnownHostsFile(sshCfg.KnownHostsFile),
					sshtool.WithWorkDir(ctx.WorkspaceDir),
				)
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
				Name:        "javascript",
				DisplayName: "自定义工具",
				Description: "运行用户创建的工具，可通过宿主函数组合现有能力、保存状态并发送任务通知。",
				Category:    "扩展",
				Builtin:     false,
			},
			Factory: func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
				current := config.Get()
				if override := ctx.Config; override != nil {
					current = override
				}
				definitions := current.JavaScript.Tools
				result := make([]runtimeport.Tool, 0, len(definitions))
				for _, definition := range definitions {
					if !definition.Enabled {
						continue
					}
					tool, err := javascripttool.NewToolWithOptions(definition, javascriptHost.options())
					if err != nil {
						return nil, fmt.Errorf("initialize javascript tool %s: %w", definition.ID, err)
					}
					result = append(result, tool)
				}
				return result, nil
			},
		},
		{
			Info: apptools.ToolGroupInfo{
				Name:          "scheduler",
				DisplayName:   "定时任务",
				Description:   "创建、查看和管理自然语言定时任务，适合提醒、周期执行和后台任务。",
				Category:      "自动化",
				Builtin:       true,
				IncludedTools: []string{"schedule_add", "schedule_list", "schedule_update", "schedule_result", "schedule_cancel", "schedule_delete"},
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

type commandManagers struct {
	mu        sync.Mutex
	byCleaner map[*resources.Cleaner]*commandtool.Manager
}

func commandToolGroup(mode commandtool.ApprovalMode, pool *commandManagers) apptools.ToolGroupFactory {
	return func(ctx apptools.ToolResolveContext) ([]runtimeport.Tool, error) {
		pool.mu.Lock()
		manager := pool.byCleaner[ctx.Cleaner]
		if manager == nil || manager.Closed() {
			manager = commandtool.NewManager()
			pool.byCleaner[ctx.Cleaner] = manager
			if ctx.Cleaner != nil {
				ctx.Cleaner.Add(manager.Close)
			}
		}
		pool.mu.Unlock()
		return commandtool.NewCommandTools(ctx.WorkspaceDir, commandtool.WithManager(manager), commandtool.WithApprovalMode(mode)).GetTools()
	}
}

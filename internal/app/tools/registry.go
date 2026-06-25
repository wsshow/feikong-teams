package tools

import (
	"fmt"
	"sync"

	"fkteams/internal/app/appdata"
	"fkteams/internal/app/tools/ask"
	"fkteams/internal/app/tools/command"
	"fkteams/internal/app/tools/doc"
	"fkteams/internal/app/tools/excel"
	"fkteams/internal/app/tools/fetch"
	"fkteams/internal/app/tools/file"
	"fkteams/internal/app/tools/script/bun"
	"fkteams/internal/app/tools/script/uv"
	"fkteams/internal/app/tools/search"
	"fkteams/internal/app/tools/todo"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

type ToolGroupFactory func(cleaner *resources.Cleaner) ([]runtimeport.Tool, error)

type ToolGroupRegistration struct {
	Info    ToolGroupInfo
	Factory ToolGroupFactory
}

type ToolGroupRegistry struct {
	mu     sync.RWMutex
	order  []string
	groups map[string]toolGroupEntry
	frozen bool
}

type toolGroupEntry struct {
	info    ToolGroupInfo
	factory ToolGroupFactory
}

func NewToolGroupRegistry() *ToolGroupRegistry {
	return &ToolGroupRegistry{groups: make(map[string]toolGroupEntry)}
}

func (r *ToolGroupRegistry) Register(reg ToolGroupRegistration) error {
	if r == nil {
		return fmt.Errorf("tool group registry is nil")
	}
	info := cloneToolGroupInfo(reg.Info)
	if info.Name == "" {
		return fmt.Errorf("tool group name is empty")
	}
	if reg.Factory == nil {
		return fmt.Errorf("tool group %s factory is nil", info.Name)
	}
	if info.DisplayName == "" {
		return fmt.Errorf("tool group %s display name is empty", info.Name)
	}
	if info.Description == "" {
		return fmt.Errorf("tool group %s description is empty", info.Name)
	}
	if info.Category == "" {
		return fmt.Errorf("tool group %s category is empty", info.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return fmt.Errorf("tool group registry is frozen")
	}
	if _, exists := r.groups[info.Name]; exists {
		return fmt.Errorf("tool group %s already registered", info.Name)
	}
	r.groups[info.Name] = toolGroupEntry{info: info, factory: reg.Factory}
	r.order = append(r.order, info.Name)
	return nil
}

func (r *ToolGroupRegistry) Resolve(name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("tool group registry is nil")
	}
	r.mu.RLock()
	entry, ok := r.groups[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	tools, err := entry.factory(cleaner)
	if err != nil {
		return nil, true, err
	}
	return tools, true, nil
}

func (r *ToolGroupRegistry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

func (r *ToolGroupRegistry) Infos() []ToolGroupInfo {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ToolGroupInfo, 0, len(r.order))
	for _, name := range r.order {
		entry, ok := r.groups[name]
		if !ok {
			continue
		}
		infos = append(infos, cloneToolGroupInfo(entry.info))
	}
	return infos
}

func (r *ToolGroupRegistry) Freeze() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

func defaultToolGroupRegistry() *ToolGroupRegistry {
	registry := NewToolGroupRegistry()
	for _, reg := range builtinToolGroups() {
		if err := registry.Register(reg); err != nil {
			panic(err)
		}
	}
	return registry
}

func builtinToolGroups() []ToolGroupRegistration {
	return []ToolGroupRegistration{
		{Info: ToolGroupInfo{
			Name:          "file",
			DisplayName:   "文件",
			Description:   "读取、搜索、创建和修改工作区文件，适合代码编辑、项目检查和文档整理。",
			Category:      "文件",
			Builtin:       true,
			IncludedTools: []string{"file_read", "file_write", "file_search", "file_list"},
		}, Factory: fileToolGroup},
		{Info: ToolGroupInfo{
			Name:          "excel",
			DisplayName:   "Excel",
			Description:   "创建、读取和编辑 Excel 工作簿，处理表格数据、公式、样式和工作表。",
			Category:      "数据",
			Builtin:       true,
			IncludedTools: []string{"excel_create", "excel_read", "excel_write", "excel_style"},
		}, Factory: excelToolGroup},
		{Info: ToolGroupInfo{
			Name:          "todo",
			DisplayName:   "待办事项",
			Description:   "管理任务清单和执行进度，适合长任务拆解、跟踪和复盘。",
			Category:      "协作",
			Builtin:       true,
			IncludedTools: []string{"todo_create", "todo_update", "todo_list"},
		}, Factory: todoToolGroup},
		{Info: ToolGroupInfo{
			Name:          "command",
			DisplayName:   "命令执行",
			Description:   "在工作区运行 shell 命令，危险操作会进入安全审批流程。",
			Category:      "开发",
			Builtin:       true,
			IncludedTools: []string{"execute"},
		}, Factory: commandToolGroup},
		{Info: ToolGroupInfo{
			Name:          "search",
			DisplayName:   "网络搜索",
			Description:   "检索互联网信息，适合需要时效性、外部资料或交叉验证的问题。",
			Category:      "研究",
			Builtin:       true,
			IncludedTools: []string{"search"},
		}, Factory: searchToolGroup},
		{Info: ToolGroupInfo{
			Name:          "fetch",
			DisplayName:   "网页抓取",
			Description:   "读取指定 URL 的网页内容，适合打开搜索结果、文档页面和公开资料。",
			Category:      "研究",
			Builtin:       true,
			IncludedTools: []string{"fetch"},
		}, Factory: fetchToolGroup},
		{Info: ToolGroupInfo{
			Name:          "doc",
			DisplayName:   "文档",
			Description:   "读取和分析文档文件，支持文档信息、智能读取、按页和按行读取。",
			Category:      "文档",
			Builtin:       true,
			IncludedTools: []string{"doc_info", "doc_smart_read", "doc_read_pages", "doc_read_lines"},
		}, Factory: docToolGroup},
		{Info: ToolGroupInfo{
			Name:          "ask",
			DisplayName:   "向用户提问",
			Description:   "允许智能体在信息不足或需要选择时向用户提问，并等待用户回答后继续。",
			Category:      "协作",
			Builtin:       true,
			IncludedTools: []string{"ask_questions"},
		}, Factory: askToolGroup},
		{Info: ToolGroupInfo{
			Name:          "uv",
			DisplayName:   "Python 脚本",
			Description:   "使用 uv 管理隔离的 Python 环境，安装依赖并运行脚本或代码片段。",
			Category:      "脚本",
			Builtin:       true,
			IncludedTools: []string{"uv_run_script", "uv_run_code", "uv_install_package"},
		}, Factory: uvToolGroup},
		{Info: ToolGroupInfo{
			Name:          "bun",
			DisplayName:   "JavaScript 脚本",
			Description:   "使用 Bun 管理 JavaScript/TypeScript 运行环境，安装依赖并执行脚本。",
			Category:      "脚本",
			Builtin:       true,
			IncludedTools: []string{"bun_run_script", "bun_run_code", "bun_install_package"},
		}, Factory: bunToolGroup},
	}
}

var defaultRegistry = defaultToolGroupRegistry()

func RegisterToolGroup(reg ToolGroupRegistration) error {
	return defaultRegistry.Register(reg)
}

func fileToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	fileTools, err := file.NewFileTools(workspacePath())
	if err != nil {
		return nil, fmt.Errorf("初始化文件工具失败: %w", err)
	}
	return fileTools.GetTools()
}

func excelToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	excelTools, err := excel.NewExcelTools(workspacePath())
	if err != nil {
		return nil, fmt.Errorf("初始化Excel工具失败: %w", err)
	}
	return excelTools.GetTools()
}

func todoToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	todoTools, err := todo.NewTodoTools(appdata.SessionsDir())
	if err != nil {
		return nil, fmt.Errorf("初始化Todo工具失败: %w", err)
	}
	return todoTools.GetTools()
}

func commandToolGroup(cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	if cleaner != nil {
		cleaner.Add(func() error {
			command.TerminateAll()
			command.CleanupTempFiles(workspacePath())
			return nil
		})
	}
	return command.NewCommandTools(workspacePath()).GetTools()
}

func searchToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	return search.GetTools()
}

func fetchToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	return fetch.GetTools()
}

func docToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	return doc.GetTools()
}

func askToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	return ask.GetTools()
}

func uvToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	uvTools, err := uv.NewUVTools(runtimeDir(), workspacePath())
	if err != nil {
		return nil, fmt.Errorf("初始化 uv 工具失败: %w", err)
	}
	return uvTools.GetTools()
}

func bunToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	bunTools, err := bun.NewBunTools(runtimeDir(), workspacePath())
	if err != nil {
		return nil, fmt.Errorf("初始化 bun 工具失败: %w", err)
	}
	return bunTools.GetTools()
}

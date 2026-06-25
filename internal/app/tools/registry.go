package tools

import (
	"fmt"
	"sync"

	schedulertool "fkteams/internal/adapters/tools/builtin/scheduler"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/config"
	"fkteams/internal/app/tools/ask"
	"fkteams/internal/app/tools/command"
	"fkteams/internal/app/tools/doc"
	"fkteams/internal/app/tools/excel"
	"fkteams/internal/app/tools/fetch"
	"fkteams/internal/app/tools/file"
	"fkteams/internal/app/tools/git"
	"fkteams/internal/app/tools/script/bun"
	"fkteams/internal/app/tools/script/uv"
	"fkteams/internal/app/tools/search"
	"fkteams/internal/app/tools/ssh"
	"fkteams/internal/app/tools/todo"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/resources"
)

type ToolGroupFactory func(cleaner *resources.Cleaner) ([]runtimeport.Tool, error)

type ToolGroupRegistration struct {
	Name    string
	Factory ToolGroupFactory
}

type ToolGroupRegistry struct {
	mu     sync.RWMutex
	order  []string
	groups map[string]ToolGroupFactory
	frozen bool
}

func NewToolGroupRegistry() *ToolGroupRegistry {
	return &ToolGroupRegistry{groups: make(map[string]ToolGroupFactory)}
}

func (r *ToolGroupRegistry) Register(reg ToolGroupRegistration) error {
	if r == nil {
		return fmt.Errorf("tool group registry is nil")
	}
	if reg.Name == "" {
		return fmt.Errorf("tool group name is empty")
	}
	if reg.Factory == nil {
		return fmt.Errorf("tool group %s factory is nil", reg.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return fmt.Errorf("tool group registry is frozen")
	}
	if _, exists := r.groups[reg.Name]; exists {
		return fmt.Errorf("tool group %s already registered", reg.Name)
	}
	r.groups[reg.Name] = reg.Factory
	r.order = append(r.order, reg.Name)
	return nil
}

func (r *ToolGroupRegistry) Resolve(name string, cleaner *resources.Cleaner) ([]runtimeport.Tool, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("tool group registry is nil")
	}
	r.mu.RLock()
	factory, ok := r.groups[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	tools, err := factory(cleaner)
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
	registry.Freeze()
	return registry
}

func builtinToolGroups() []ToolGroupRegistration {
	return []ToolGroupRegistration{
		{Name: "file", Factory: fileToolGroup},
		{Name: "git", Factory: gitToolGroup},
		{Name: "excel", Factory: excelToolGroup},
		{Name: "todo", Factory: todoToolGroup},
		{Name: "ssh", Factory: sshToolGroup},
		{Name: "command", Factory: commandToolGroup},
		{Name: "scheduler", Factory: schedulerToolGroup},
		{Name: "search", Factory: searchToolGroup},
		{Name: "fetch", Factory: fetchToolGroup},
		{Name: "doc", Factory: docToolGroup},
		{Name: "ask", Factory: askToolGroup},
		{Name: "uv", Factory: uvToolGroup},
		{Name: "bun", Factory: bunToolGroup},
	}
}

var defaultRegistry = defaultToolGroupRegistry()

func fileToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	fileTools, err := file.NewFileTools(workspacePath())
	if err != nil {
		return nil, fmt.Errorf("初始化文件工具失败: %w", err)
	}
	return fileTools.GetTools()
}

func gitToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	gitTools, err := git.NewGitTools(workspacePath())
	if err != nil {
		return nil, fmt.Errorf("初始化Git工具失败: %w", err)
	}
	return gitTools.GetTools()
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

func sshToolGroup(cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	sshCfg := config.Get().Agents.SSHVisitor
	host := sshCfg.Host
	username := sshCfg.Username
	password := sshCfg.Password
	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("SSH 连接信息未配置，请在配置文件 [agents.ssh_visitor] 中设置 host, username, password")
	}
	sshTools, err := ssh.NewSSHTools(host, username, password)
	if err != nil {
		return nil, fmt.Errorf("初始化 SSH 工具失败: %w", err)
	}
	if cleaner != nil {
		cleaner.Add(func() error {
			sshTools.Close()
			return nil
		})
	}
	return sshTools.GetTools()
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

func schedulerToolGroup(*resources.Cleaner) ([]runtimeport.Tool, error) {
	return schedulertool.NewTools(nil).GetTools()
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

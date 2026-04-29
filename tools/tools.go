package tools

import (
	"context"
	"fkteams/common"
	"fkteams/config"
	"fkteams/g"
	"fkteams/tools/ask"
	"fkteams/tools/command"
	"fkteams/tools/doc"
	"fkteams/tools/excel"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	"fkteams/tools/git"
	"fkteams/tools/mcp"
	"fkteams/tools/scheduler"
	"fkteams/tools/script/bun"
	"fkteams/tools/script/uv"
	"fkteams/tools/search"
	"fkteams/tools/ssh"
	"fkteams/tools/todo"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

var cmdCleanupOnce sync.Once

// workspacePath 返回工作区目录路径
func workspacePath() string {
	return common.WorkspaceDir()
}

// runtimeDir 返回脚本运行时环境目录
func runtimeDir() string {
	return filepath.Join(common.AppDir(), "runtime")
}

func GetToolsByName(name string) ([]tool.BaseTool, error) {
	switch name {
	case "file":
		fileTools, err := file.NewFileTools(workspacePath())
		if err != nil {
			return nil, fmt.Errorf("初始化文件工具失败: %w", err)
		}
		return fileTools.GetTools()
	case "git":
		gitTools, err := git.NewGitTools(workspacePath())
		if err != nil {
			return nil, fmt.Errorf("初始化Git工具失败: %w", err)
		}
		return gitTools.GetTools()
	case "excel":
		excelTools, err := excel.NewExcelTools(workspacePath())
		if err != nil {
			return nil, fmt.Errorf("初始化Excel工具失败: %w", err)
		}
		return excelTools.GetTools()
	case "todo":
		todoTools, err := todo.NewTodoTools(common.SessionsDir())
		if err != nil {
			return nil, fmt.Errorf("初始化Todo工具失败: %w", err)
		}
		return todoTools.GetTools()
	case "ssh":
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
		g.ProcessCleaner.Add(func() error {
			sshTools.Close()
			return nil
		})
		return sshTools.GetTools()
	case "command":
		cmdCleanupOnce.Do(func() {
			g.ProcessCleaner.Add(func() error {
				command.TerminateAll()
				command.CleanupTempFiles(workspacePath())
				return nil
			})
		})
		return command.NewCommandTools(workspacePath()).GetTools()
	case "scheduler":
		s, err := scheduler.InitGlobal(common.SchedulerDir())
		if err != nil {
			return nil, fmt.Errorf("初始化调度器工具失败: %w", err)
		}
		return s.GetTools()
	case "search":
		duckduckgoTool, err := search.NewDuckDuckGoTool(context.Background())
		return []tool.BaseTool{duckduckgoTool}, err
	case "fetch":
		return fetch.GetTools()
	case "doc":
		return doc.GetTools()
	case "ask":
		return ask.GetTools()
	case "uv":
		uvTools, err := uv.NewUVTools(runtimeDir(), workspacePath())
		if err != nil {
			return nil, fmt.Errorf("初始化 uv 工具失败: %w", err)
		}
		return uvTools.GetTools()
	case "bun":
		bunTools, err := bun.NewBunTools(runtimeDir(), workspacePath())
		if err != nil {
			return nil, fmt.Errorf("初始化 bun 工具失败: %w", err)
		}
		return bunTools.GetTools()
	default:
		if name, ok := strings.CutPrefix(name, "mcp-"); ok {
			return mcp.GetToolsByName(name)
		}
		return nil, fmt.Errorf("tool %s not found", name)
	}
}

// BuiltinToolNames 返回所有内置工具组名称
func BuiltinToolNames() []string {
	return []string{
		"file", "git", "excel", "todo", "ssh",
		"command", "scheduler", "search", "fetch", "doc",
		"ask", "uv", "bun",
	}
}

// GetAllToolNames 返回所有可用的工具名列表（内置 + MCP）
func GetAllToolNames() []string {
	names := make([]string, 0, len(BuiltinToolNames()))
	names = append(names, BuiltinToolNames()...)
	mcpGroups, err := mcp.GetAllToolGroups()
	if err == nil {
		for name := range mcpGroups {
			names = append(names, "mcp-"+name)
		}
	}
	return names
}

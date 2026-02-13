package tools

import (
	"context"
	"fkteams/g"
	"fkteams/tools/command"
	"fkteams/tools/doc"
	"fkteams/tools/excel"
	"fkteams/tools/fetch"
	"fkteams/tools/file"
	"fkteams/tools/git"
	"fkteams/tools/mcp"
	"fkteams/tools/script/bun"
	"fkteams/tools/script/uv"
	"fkteams/tools/search"
	"fkteams/tools/ssh"
	"fkteams/tools/todo"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
)

func GetToolsByName(name string) ([]tool.BaseTool, error) {
	switch name {
	case "file":
		safeDir := "./workspace"
		codeDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if codeDirEnv != "" {
			safeDir = codeDirEnv
		}
		fileTools, err := file.NewFileTools(safeDir)
		if err != nil {
			return nil, fmt.Errorf("初始化文件工具失败: %w", err)
		}
		return fileTools.GetTools()
	case "git":
		gitDir := "./workspace"
		gitDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if gitDirEnv != "" {
			gitDir = gitDirEnv
		}
		gitTools, err := git.NewGitTools(gitDir)
		if err != nil {
			return nil, fmt.Errorf("初始化Git工具失败: %w", err)
		}
		return gitTools.GetTools()
	case "excel":
		excelDir := "./workspace"
		excelDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if excelDirEnv != "" {
			excelDir = excelDirEnv
		}
		excelTools, err := excel.NewExcelTools(excelDir)
		if err != nil {
			return nil, fmt.Errorf("初始化Excel工具失败: %w", err)
		}
		return excelTools.GetTools()
	case "todo":
		todoDir := "./workspace"
		todoDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if todoDirEnv != "" {
			todoDir = todoDirEnv
		}
		todoTools, err := todo.NewTodoTools(todoDir)
		if err != nil {
			return nil, fmt.Errorf("初始化Todo工具失败: %w", err)
		}
		return todoTools.GetTools()
	case "ssh":
		host := os.Getenv("FEIKONG_SSH_HOST")
		username := os.Getenv("FEIKONG_SSH_USERNAME")
		password := os.Getenv("FEIKONG_SSH_PASSWORD")
		if host == "" || username == "" || password == "" {
			return nil, fmt.Errorf("SSH 连接信息未配置。请设置以下环境变量：FEIKONG_SSH_HOST, FEIKONG_SSH_USERNAME, FEIKONG_SSH_PASSWORD")
		}
		sshTools, err := ssh.NewSSHTools(host, username, password)
		if err != nil {
			return nil, fmt.Errorf("初始化 SSH 工具失败: %w", err)
		}
		g.Cleaner.Add(func() error {
			sshTools.Close()
			return nil
		})
		return sshTools.GetTools()
	case "command":
		return command.GetTools()
	case "search":
		duckduckgoTool, err := search.NewDuckDuckGoTool(context.Background())
		return []tool.BaseTool{duckduckgoTool}, err
	case "fetch":
		return fetch.GetTools()
	case "doc":
		return doc.GetTools()
	case "uv":
		uvDir := "./workspace"
		uvDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if uvDirEnv != "" {
			uvDir = uvDirEnv
		}
		uvTools, err := uv.NewUVTools(uvDir)
		if err != nil {
			return nil, fmt.Errorf("初始化 uv 工具失败: %w", err)
		}
		return uvTools.GetTools()
	case "bun":
		bunDir := "./workspace"
		bunDirEnv := os.Getenv("FEIKONG_WORKSPACE_DIR")
		if bunDirEnv != "" {
			bunDir = bunDirEnv
		}
		bunTools, err := bun.NewBunTools(bunDir)
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

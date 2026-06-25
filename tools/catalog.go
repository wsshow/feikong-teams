package tools

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/tools/mcp"
)

// ToolGroupInfo 描述可在自定义智能体中配置的工具组。
type ToolGroupInfo struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Builtin       bool     `json:"builtin"`
	IncludedTools []string `json:"included_tools,omitempty"`
}

var builtinToolCatalog = []ToolGroupInfo{
	{
		Name:          "file",
		DisplayName:   "文件",
		Description:   "读取、搜索、创建和修改工作区文件，适合代码编辑、项目检查和文档整理。",
		Category:      "文件",
		Builtin:       true,
		IncludedTools: []string{"file_read", "file_write", "file_search", "file_list"},
	},
	{
		Name:          "git",
		DisplayName:   "Git",
		Description:   "查看状态、提交、分支、日志和差异，适合版本管理和代码变更检查。",
		Category:      "开发",
		Builtin:       true,
		IncludedTools: []string{"git_status", "git_diff", "git_log", "git_commit"},
	},
	{
		Name:          "excel",
		DisplayName:   "Excel",
		Description:   "创建、读取和编辑 Excel 工作簿，处理表格数据、公式、样式和工作表。",
		Category:      "数据",
		Builtin:       true,
		IncludedTools: []string{"excel_create", "excel_read", "excel_write", "excel_style"},
	},
	{
		Name:          "todo",
		DisplayName:   "待办事项",
		Description:   "管理任务清单和执行进度，适合长任务拆解、跟踪和复盘。",
		Category:      "协作",
		Builtin:       true,
		IncludedTools: []string{"todo_create", "todo_update", "todo_list"},
	},
	{
		Name:          "ssh",
		DisplayName:   "SSH",
		Description:   "连接远程服务器执行命令、上传下载文件和查看目录，需要先配置 SSH 连接信息。",
		Category:      "运维",
		Builtin:       true,
		IncludedTools: []string{"ssh_execute", "ssh_upload", "ssh_download", "ssh_list_dir"},
	},
	{
		Name:          "command",
		DisplayName:   "命令执行",
		Description:   "在工作区运行 shell 命令，危险操作会进入安全审批流程。",
		Category:      "开发",
		Builtin:       true,
		IncludedTools: []string{"execute"},
	},
	{
		Name:          "scheduler",
		DisplayName:   "定时任务",
		Description:   "创建、查看和管理自然语言定时任务，适合提醒、周期执行和后台任务。",
		Category:      "自动化",
		Builtin:       true,
		IncludedTools: []string{"schedule_create", "schedule_list", "schedule_cancel"},
	},
	{
		Name:          "search",
		DisplayName:   "网络搜索",
		Description:   "检索互联网信息，适合需要时效性、外部资料或交叉验证的问题。",
		Category:      "研究",
		Builtin:       true,
		IncludedTools: []string{"search"},
	},
	{
		Name:          "fetch",
		DisplayName:   "网页抓取",
		Description:   "读取指定 URL 的网页内容，适合打开搜索结果、文档页面和公开资料。",
		Category:      "研究",
		Builtin:       true,
		IncludedTools: []string{"fetch"},
	},
	{
		Name:          "doc",
		DisplayName:   "文档",
		Description:   "读取和分析文档文件，支持文档信息、智能读取、按页和按行读取。",
		Category:      "文档",
		Builtin:       true,
		IncludedTools: []string{"doc_info", "doc_smart_read", "doc_read_pages", "doc_read_lines"},
	},
	{
		Name:          "ask",
		DisplayName:   "向用户提问",
		Description:   "允许智能体在信息不足或需要选择时向用户提问，并等待用户回答后继续。",
		Category:      "协作",
		Builtin:       true,
		IncludedTools: []string{"ask_questions"},
	},
	{
		Name:          "uv",
		DisplayName:   "Python 脚本",
		Description:   "使用 uv 管理隔离的 Python 环境，安装依赖并运行脚本或代码片段。",
		Category:      "脚本",
		Builtin:       true,
		IncludedTools: []string{"uv_run_script", "uv_run_code", "uv_install_package"},
	},
	{
		Name:          "bun",
		DisplayName:   "JavaScript 脚本",
		Description:   "使用 Bun 管理 JavaScript/TypeScript 运行环境，安装依赖并执行脚本。",
		Category:      "脚本",
		Builtin:       true,
		IncludedTools: []string{"bun_run_script", "bun_run_code", "bun_install_package"},
	},
}

// BuiltinToolInfos 返回内置可配置工具组信息。
func BuiltinToolInfos() []ToolGroupInfo {
	infos := make([]ToolGroupInfo, len(builtinToolCatalog))
	copy(infos, builtinToolCatalog)
	for i := range infos {
		infos[i].IncludedTools = append([]string(nil), infos[i].IncludedTools...)
	}
	return infos
}

// GetAllToolInfos 返回所有可配置工具组信息（内置 + MCP）。
func GetAllToolInfos() []ToolGroupInfo {
	infos := BuiltinToolInfos()
	mcpGroups, err := mcp.GetAllToolGroups()
	if err != nil {
		return infos
	}
	for name, group := range mcpGroups {
		info := ToolGroupInfo{
			Name:          "mcp-" + name,
			DisplayName:   "MCP: " + name,
			Description:   group.Desc,
			Category:      "MCP",
			Builtin:       false,
			IncludedTools: toolNames(group.Tools),
		}
		if info.Description == "" {
			info.Description = "来自 MCP 服务 " + name + " 的工具组。"
		}
		infos = append(infos, info)
	}
	return infos
}

func toolNames(list []runtimeport.Tool) []string {
	names := make([]string, 0, len(list))
	for _, t := range list {
		info, err := t.Info(context.Background())
		if err == nil && info != nil && info.Name != "" {
			names = append(names, info.Name)
		}
	}
	return names
}

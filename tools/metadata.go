package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
)

const (
	metaReadOnly    = "fkteams:readOnly"
	metaDestructive = "fkteams:destructive"
)

// readOnlyToolNames 只读工具名称集合
var readOnlyToolNames = map[string]bool{
	// file
	"file_read": true,
	"grep":      true,
	"file_list": true,
	"glob":      true,
	// git
	"git_status":  true,
	"git_log":     true,
	"git_diff":    true,
	"git_branch":  true,
	"git_tag":     true,
	"git_remote":  true,
	"git_config":  true,
	"git_clean":   true,
	// search
	"search": true,
	"fetch":  true,
	// ssh
	"ssh_list_dir":  true,
	"ssh_download":  true,
	// doc
	"doc_read": true,
}

// destructiveToolNames 破坏性工具名称集合
var destructiveToolNames = map[string]bool{
	// file
	"file_write":  true,
	"file_append": true,
	"file_edit":   true,
	"file_patch":  true,
	// git
	"git_init":    true,
	"git_add":     true,
	"git_commit":  true,
	"git_checkout": true,
	"git_reset":   true,
	"git_remove":  true,
	// command
	"execute": true,
	// ssh
	"ssh_execute": true,
	"ssh_upload":  true,
	// script
	"bun": true,
	"uv":  true,
	// scheduler
	"schedule_add":    true,
	"schedule_cancel": true,
	"schedule_delete": true,
}

// IsReadOnlyTool 判断工具是否只读
func IsReadOnlyTool(toolName string) bool {
	return readOnlyToolNames[toolName]
}

// IsDestructiveTool 判断工具是否有破坏性
func IsDestructiveTool(toolName string) bool {
	return destructiveToolNames[toolName]
}

// ClassifyTool 为工具设置只读/破坏性元数据
func ClassifyTool(t tool.BaseTool) {
	info, err := t.Info(context.Background())
	if err != nil {
		return
	}
	if info.Extra == nil {
		info.Extra = make(map[string]any)
	}
	name := info.Name
	if IsReadOnlyTool(name) {
		info.Extra[metaReadOnly] = true
	}
	if IsDestructiveTool(name) {
		info.Extra[metaDestructive] = true
	}
}

// ClassifyTools 批量为工具列表设置元数据
func ClassifyTools(tools []tool.BaseTool) {
	for _, t := range tools {
		ClassifyTool(t)
	}
}

// SetTestClassification 测试辅助：动态设置工具分类
func SetTestClassification(name string, readOnly, destructive bool) {
	if readOnly {
		readOnlyToolNames[name] = true
	}
	if destructive {
		destructiveToolNames[name] = true
	}
}

package tools

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/tools/approval"
	"fmt"
)

const (
	metaReadOnly       = "fkteams:readOnly"
	metaDestructive    = "fkteams:destructive"
	metaSerialize      = "fkteams:serialize"
	metaApprovalStore  = "fkteams:approvalStore"
	metaExternalPath   = "fkteams:externalPath"
	metaPolicyRequired = "fkteams:policyRequired"
)

type ToolPolicy struct {
	ReadOnly      bool
	Destructive   bool
	Serialize     bool
	ApprovalStore string
	ExternalPath  bool
}

var toolPolicies = map[string]ToolPolicy{
	// 文件
	"file_read":   readOnlyPolicy(approval.StoreFile, true),
	"grep":        readOnlyPolicy(approval.StoreFile, true),
	"file_list":   readOnlyPolicy(approval.StoreFile, true),
	"glob":        readOnlyPolicy(approval.StoreFile, true),
	"file_write":  destructivePolicy(approval.StoreFile, true),
	"file_append": destructivePolicy(approval.StoreFile, true),
	"file_edit":   destructivePolicy(approval.StoreFile, true),
	"file_patch":  destructivePolicy(approval.StoreFile, true),

	// Git 工具
	"git_status":   readOnlyPolicy("", false),
	"git_log":      readOnlyPolicy("", false),
	"git_diff":     readOnlyPolicy("", false),
	"git_init":     destructivePolicy(approval.StoreGit, false),
	"git_add":      destructivePolicy(approval.StoreGit, false),
	"git_commit":   destructivePolicy(approval.StoreGit, false),
	"git_checkout": destructivePolicy(approval.StoreGit, false),
	"git_reset":    destructivePolicy(approval.StoreGit, false),
	"git_remove":   destructivePolicy(approval.StoreGit, false),
	"git_branch":   destructivePolicy(approval.StoreGit, false),
	"git_tag":      destructivePolicy(approval.StoreGit, false),
	"git_remote":   destructivePolicy(approval.StoreGit, false),
	"git_config":   destructivePolicy(approval.StoreGit, false),
	"git_clean":    destructivePolicy(approval.StoreGit, false),

	// 命令和分发
	"execute":        destructivePolicy(approval.StoreCommand, false),
	"dispatch_tasks": destructivePolicy(approval.StoreDispatch, false),

	// SSH 工具
	"ssh_list_dir": readOnlyPolicy("", false),
	"ssh_download": destructivePolicy("", false),
	"ssh_execute":  destructivePolicy("", false),
	"ssh_upload":   destructivePolicy("", false),

	// Bun 脚本
	"bun_init_env":        destructivePolicy("", false),
	"bun_install_package": destructivePolicy("", false),
	"bun_remove_package":  destructivePolicy("", false),
	"bun_clean_env":       destructivePolicy("", false),
	"bun_run_script":      destructivePolicy("", false),
	"bun_list_package":    readOnlyPolicy("", false),
	"bun_get_env_info":    readOnlyPolicy("", false),

	// uv 脚本
	"uv_init_env":        destructivePolicy("", false),
	"uv_install_package": destructivePolicy("", false),
	"uv_remove_package":  destructivePolicy("", false),
	"uv_clean_env":       destructivePolicy("", false),
	"uv_run_script":      destructivePolicy("", false),
	"uv_run_code":        destructivePolicy("", false),
	"uv_format_code":     destructivePolicy("", false),
	"uv_list_package":    readOnlyPolicy("", false),
	"uv_get_env_info":    readOnlyPolicy("", false),
	"uv_check_syntax":    readOnlyPolicy("", false),

	// 调度器
	"schedule_list":   readOnlyPolicy("", false),
	"schedule_add":    destructivePolicy("", false),
	"schedule_cancel": destructivePolicy("", false),
	"schedule_delete": destructivePolicy("", false),

	// TODO 工具
	"todo_list":         readOnlyPolicy("", false),
	"todo_add":          destructivePolicy("", false),
	"todo_update":       destructivePolicy("", false),
	"todo_delete":       destructivePolicy("", false),
	"todo_batch_add":    destructivePolicy("", false),
	"todo_batch_delete": destructivePolicy("", false),
	"todo_clear":        destructivePolicy("", false),

	// 搜索、抓取、文档和提问
	"search":                  readOnlyPolicy("", false),
	"fetch":                   readOnlyPolicy("", false),
	"get_document_info":       readOnlyPolicy("", false),
	"read_document_smart":     readOnlyPolicy("", false),
	"read_document_by_pages":  readOnlyPolicy("", false),
	"read_document_by_lines":  readOnlyPolicy("", false),
	"ask_questions":           readOnlyPolicy("", false),
	"session_attachment_list": readOnlyPolicy("", false),
	"session_attachment_read": readOnlyPolicy("", false),

	// Excel 读取
	"excel_open_workbook":       readOnlyPolicy("", false),
	"excel_get_cell_value":      readOnlyPolicy("", false),
	"excel_get_rows":            readOnlyPolicy("", false),
	"excel_get_row":             readOnlyPolicy("", false),
	"excel_get_col":             readOnlyPolicy("", false),
	"excel_get_sheet_data":      readOnlyPolicy("", false),
	"excel_get_all_sheets_data": readOnlyPolicy("", false),

	// Excel 写入
	"excel_create_workbook":        destructivePolicy("", false),
	"excel_save_workbook":          destructivePolicy("", false),
	"excel_set_workbook_props":     destructivePolicy("", false),
	"excel_create_sheet":           destructivePolicy("", false),
	"excel_delete_sheet":           destructivePolicy("", false),
	"excel_rename_sheet":           destructivePolicy("", false),
	"excel_copy_sheet":             destructivePolicy("", false),
	"excel_set_active_sheet":       destructivePolicy("", false),
	"excel_set_sheet_visible":      destructivePolicy("", false),
	"excel_set_cell_value":         destructivePolicy("", false),
	"excel_set_cell_formula":       destructivePolicy("", false),
	"excel_merge_cells":            destructivePolicy("", false),
	"excel_unmerge_cells":          destructivePolicy("", false),
	"excel_set_cell_style":         destructivePolicy("", false),
	"excel_set_row_height":         destructivePolicy("", false),
	"excel_set_col_width":          destructivePolicy("", false),
	"excel_insert_row":             destructivePolicy("", false),
	"excel_remove_row":             destructivePolicy("", false),
	"excel_insert_col":             destructivePolicy("", false),
	"excel_remove_col":             destructivePolicy("", false),
	"excel_create_style":           destructivePolicy("", false),
	"excel_set_conditional_format": destructivePolicy("", false),
	"excel_add_picture":            destructivePolicy("", false),
	"excel_add_chart":              destructivePolicy("", false),
	"excel_batch_create_sheets":    destructivePolicy("", false),
	"excel_batch_delete_sheets":    destructivePolicy("", false),
	"excel_batch_set_cell_values":  destructivePolicy("", false),
	"excel_batch_fill_rows":        destructivePolicy("", false),
	"excel_batch_fill_cols":        destructivePolicy("", false),
	"excel_batch_insert_rows":      destructivePolicy("", false),
	"excel_batch_remove_rows":      destructivePolicy("", false),
	"excel_batch_insert_cols":      destructivePolicy("", false),
	"excel_batch_remove_cols":      destructivePolicy("", false),
	"excel_batch_set_cell_styles":  destructivePolicy("", false),
}

func readOnlyPolicy(approvalStore string, externalPath bool) ToolPolicy {
	return ToolPolicy{
		ReadOnly:      true,
		ApprovalStore: approvalStore,
		ExternalPath:  externalPath,
	}
}

func destructivePolicy(approvalStore string, externalPath bool) ToolPolicy {
	return ToolPolicy{
		Destructive:   true,
		Serialize:     true,
		ApprovalStore: approvalStore,
		ExternalPath:  externalPath,
	}
}

func PolicyForTool(toolName string) (ToolPolicy, bool) {
	policy, ok := toolPolicies[toolName]
	return policy, ok
}

func ShouldSerializeTool(toolName string) bool {
	policy, ok := PolicyForTool(toolName)
	return ok && policy.Serialize
}

// MarkPolicyRequired 标记工具必须在策略表中声明安全策略。
func MarkPolicyRequired(tools []runtimeport.Tool) error {
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			return err
		}
		if info.Extra == nil {
			info.Extra = make(map[string]any)
		}
		info.Extra[metaPolicyRequired] = true
	}
	return nil
}

// ClassifyTool 为工具设置策略元数据
func ClassifyTool(t runtimeport.Tool) error {
	info, err := t.Info(context.Background())
	if err != nil {
		return err
	}
	if info.Extra == nil {
		info.Extra = make(map[string]any)
	}
	policyRequired, _ := info.Extra[metaPolicyRequired].(bool)

	policy, ok := PolicyForTool(info.Name)
	if !ok {
		if policyRequired {
			return fmt.Errorf("missing tool policy: %s", info.Name)
		}
		return nil
	}
	if policy.ReadOnly {
		info.Extra[metaReadOnly] = true
	}
	if policy.Destructive {
		info.Extra[metaDestructive] = true
	}
	if policy.Serialize {
		info.Extra[metaSerialize] = true
	}
	if policy.ApprovalStore != "" {
		info.Extra[metaApprovalStore] = policy.ApprovalStore
	}
	if policy.ExternalPath {
		info.Extra[metaExternalPath] = true
	}
	delete(info.Extra, metaPolicyRequired)
	return nil
}

// ClassifyTools 批量为工具列表设置元数据
func ClassifyTools(tools []runtimeport.Tool) error {
	for _, t := range tools {
		if err := ClassifyTool(t); err != nil {
			return err
		}
	}
	return nil
}

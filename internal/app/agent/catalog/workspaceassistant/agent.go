// Package workspaceassistant 定义面向用户的个人助手。
package workspaceassistant

import (
	"fkteams/internal/app/agent/catalog/common"
	domainsession "fkteams/internal/domain/session"
)

// DefaultDefinition 返回个人助手的默认能力与行为声明。
func DefaultDefinition() common.Definition {
	return common.Definition{
		Name:        domainsession.AssistantAgent,
		Description: "个人助手，结合用户背景处理问题、资料与任务，持续推进并交付结果。",
		Instruction: assistantPrompt + "\n" + openUIPrompt,
		Profile:     common.ProfileFull,
		ToolNames:   []string{"file", "command", "search", "fetch", "ask", "conversations", "assistant_identity", "scheduler", "memory", "tasks"},
	}
}

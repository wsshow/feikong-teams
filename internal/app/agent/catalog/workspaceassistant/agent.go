// Package workspaceassistant 定义面向用户的常驻工作区助手。
package workspaceassistant

import (
	"fkteams/internal/app/agent/catalog/common"
	domainsession "fkteams/internal/domain/session"
)

// DefaultDefinition 返回具备工作区操作和跨会话检索能力的助手声明。
func DefaultDefinition() common.Definition {
	return common.Definition{
		Name:        domainsession.AssistantAgent,
		Description: "常驻助手，通过对话处理工作区任务、检索历史会话，并记住用户设定的名字。",
		Instruction: assistantPrompt + "\n" + openUIPrompt,
		Profile:     common.ProfileFull,
		ToolNames:   []string{"file", "command", "search", "fetch", "ask", "conversations", "assistant_identity", "scheduler"},
	}
}

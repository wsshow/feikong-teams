package visitor

import (
	"fkteams/internal/app/agent/catalog/common"
)

func DefaultDefinition(sshHost, sshUsername string) common.Definition {
	return common.Definition{
		Name:        "remote",
		Description: "远程运维专家，负责通过 SSH 管理服务器、执行命令和传输文件。",
		Instruction: visitorPrompt,
		Profile:     common.ProfileWorkspace,
		TemplateVars: map[string]any{
			"ssh_host":     sshHost,
			"ssh_username": sshUsername,
		},
		ToolNames: []string{"ssh"},
	}
}

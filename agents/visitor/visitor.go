package visitor

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"fmt"

	runtimeport "fkteams/internal/ports/runtime"
)

func NewAgent(ctx context.Context) (runtimeport.Agent, error) {
	sshCfg := config.Get().Agents.SSHVisitor

	if sshCfg.Host == "" || sshCfg.Username == "" || sshCfg.Password == "" {
		return nil, fmt.Errorf("SSH 连接信息未配置，请在配置文件 [agents.ssh_visitor] 中设置 host, username, password")
	}

	fmt.Printf("[tips] SSH 访问者智能体已初始化，连接到: %s (用户: %s)\n", sshCfg.Host, sshCfg.Username)

	return common.NewAgentBuilder("remote", "远程运维专家，负责通过 SSH 管理服务器、执行命令和传输文件。").
		WithInstruction(visitorPrompt).
		WithTemplateVar("ssh_host", sshCfg.Host).
		WithTemplateVar("ssh_username", sshCfg.Username).
		WithToolNames("ssh").
		Build(ctx)
}

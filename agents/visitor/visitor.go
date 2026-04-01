package visitor

import (
	"context"
	"fkteams/agents/common"
	"fkteams/config"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	sshCfg := config.Get().Agents.SSHVisitor

	if sshCfg.Host == "" || sshCfg.Username == "" || sshCfg.Password == "" {
		return nil, fmt.Errorf("SSH 连接信息未配置，请在配置文件 [agents.ssh_visitor] 中设置 host, username, password")
	}

	fmt.Printf("[tips] SSH 访问者智能体已初始化，连接到: %s (用户: %s)\n", sshCfg.Host, sshCfg.Username)

	return common.NewAgentBuilder("小访", "远程访问专家，擅长通过 SSH 连接远程服务器，执行命令、传输文件和管理远程系统。").
		WithTemplate(visitorPromptTemplate).
		WithTemplateVar("ssh_host", sshCfg.Host).
		WithTemplateVar("ssh_username", sshCfg.Username).
		WithToolNames("ssh").
		Build(ctx)
}

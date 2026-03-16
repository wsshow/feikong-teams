package visitor

import (
	"context"
	"fkteams/agents/common"
	toolSSH "fkteams/tools/ssh"
	"fmt"
	"os"

	"github.com/cloudwego/eino/adk"
)

var globalSSHToolsInstance *toolSSH.SSHTools

func NewAgent() (adk.Agent, error) {
	host := os.Getenv("FEIKONG_SSH_HOST")
	username := os.Getenv("FEIKONG_SSH_USERNAME")
	password := os.Getenv("FEIKONG_SSH_PASSWORD")

	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("SSH 连接信息未配置。请设置以下环境变量：FEIKONG_SSH_HOST, FEIKONG_SSH_USERNAME, FEIKONG_SSH_PASSWORD")
	}

	sshToolsInstance, err := toolSSH.NewSSHTools(host, username, password)
	if err != nil {
		return nil, fmt.Errorf("init SSH tools: %w", err)
	}
	globalSSHToolsInstance = sshToolsInstance

	fmt.Printf("[tips] SSH 访问者智能体已初始化，连接到: %s (用户: %s)\n", host, username)

	sshTools, err := sshToolsInstance.GetTools()
	if err != nil {
		return nil, fmt.Errorf("create SSH tools: %w", err)
	}

	return common.NewAgentBuilder("小访", "远程访问专家，擅长通过 SSH 连接远程服务器，执行命令、传输文件和管理远程系统。").
		WithTemplate(VisitorPromptTemplate).
		WithTemplateVar("ssh_host", host).
		WithTemplateVar("ssh_username", username).
		WithTools(sshTools...).
		Build(context.Background())
}

func CloseSSHClient() {
	if globalSSHToolsInstance != nil {
		globalSSHToolsInstance.Close()
		globalSSHToolsInstance = nil
	}
}

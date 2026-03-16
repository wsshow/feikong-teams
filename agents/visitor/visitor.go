package visitor

import (
	"context"
	"fkteams/agents/common"
	"fmt"
	"os"

	"github.com/cloudwego/eino/adk"
)

func NewAgent(ctx context.Context) (adk.Agent, error) {
	host := os.Getenv("FEIKONG_SSH_HOST")
	username := os.Getenv("FEIKONG_SSH_USERNAME")

	if host == "" || username == "" || os.Getenv("FEIKONG_SSH_PASSWORD") == "" {
		return nil, fmt.Errorf("SSH 连接信息未配置。请设置以下环境变量：FEIKONG_SSH_HOST, FEIKONG_SSH_USERNAME, FEIKONG_SSH_PASSWORD")
	}

	fmt.Printf("[tips] SSH 访问者智能体已初始化，连接到: %s (用户: %s)\n", host, username)

	return common.NewAgentBuilder("小访", "远程访问专家，擅长通过 SSH 连接远程服务器，执行命令、传输文件和管理远程系统。").
		WithTemplate(visitorPromptTemplate).
		WithTemplateVar("ssh_host", host).
		WithTemplateVar("ssh_username", username).
		WithToolNames("ssh").
		Build(ctx)
}

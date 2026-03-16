package visitor

import (
	"context"
	"fkteams/agents/common"
	toolSSH "fkteams/tools/ssh"
	"fmt"
	"os"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

var globalSSHToolsInstance *toolSSH.SSHTools

func NewAgent() (adk.Agent, error) {
	ctx := context.Background()

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

	systemMessages, err := VisitorPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"ssh_host":     host,
		"ssh_username": username,
	})
	if err != nil {
		return nil, fmt.Errorf("format prompt: %w", err)
	}

	chatModel, err := common.NewChatModel()
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小访",
		Description:   "远程访问专家，擅长通过 SSH 连接远程服务器，执行命令、传输文件和管理远程系统。",
		Instruction:   systemMessages[0].Content,
		Model:         chatModel,
		MaxIterations: common.MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  common.MaxRetries,
			IsRetryAble: common.IsRetryAble,
		},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: sshTools,
			},
		},
	})
}

func CloseSSHClient() {
	if globalSSHToolsInstance != nil {
		globalSSHToolsInstance.Close()
		globalSSHToolsInstance = nil
	}
}

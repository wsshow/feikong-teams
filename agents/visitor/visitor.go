package visitor

import (
	"context"
	"fkteams/agents/common"
	toolSSH "fkteams/tools/ssh"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

var globalSSHToolsInstance *toolSSH.SSHTools

func NewAgent() adk.Agent {
	ctx := context.Background()

	// 从环境变量读取 SSH 连接信息
	host := os.Getenv("FEIKONG_SSH_HOST")
	username := os.Getenv("FEIKONG_SSH_USERNAME")
	password := os.Getenv("FEIKONG_SSH_PASSWORD")

	// 验证环境变量
	if host == "" || username == "" || password == "" {
		log.Fatal("SSH 连接信息未配置。请设置以下环境变量：FEIKONG_SSH_HOST, FEIKONG_SSH_USERNAME, FEIKONG_SSH_PASSWORD")
	}

	// 创建 SSH 工具实例
	sshToolsInstance, err := toolSSH.NewSSHTools(host, username, password)
	if err != nil {
		log.Fatalf("初始化 SSH 工具失败: %v", err)
	}

	// 保存实例以便后续关闭
	globalSSHToolsInstance = sshToolsInstance

	fmt.Printf("[tips] SSH 访问者智能体已初始化，连接到: %s (用户: %s)\n", host, username)

	// 创建 SSH 工具
	sshTools, err := sshToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建 SSH 工具失败:", err)
	}

	// 格式化系统消息
	systemMessages, err := VisitorPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"ssh_host":     host,
		"ssh_username": username,
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	// 创建智能体
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小访",
		Description:   "远程访问专家，擅长通过 SSH 连接远程服务器，执行命令、传输文件和管理远程系统。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: sshTools,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

func CloseSSHClient() {
	if globalSSHToolsInstance != nil {
		globalSSHToolsInstance.Close()
		globalSSHToolsInstance = nil
	}
}

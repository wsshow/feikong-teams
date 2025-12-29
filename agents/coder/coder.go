package coder

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()

	// 初始化安全的文件系统，限制操作在二进制文件同级的 code 目录下
	// 获取可执行文件的路径
	execPath, err := os.Executable()
	if err != nil {
		log.Fatal("无法获取可执行文件路径:", err)
	}

	// 获取可执行文件所在的目录
	execDir := filepath.Dir(execPath)

	// 设置 code 目录为可执行文件同级的 code 目录
	codeDir := filepath.Join(execDir, "code")

	// 初始化安全的文件系统
	if err := tools.InitSecuredFileSystem(codeDir); err != nil {
		log.Fatal("初始化文件系统失败:", err)
	}

	log.Printf("文件工具已限制在目录: %s", codeDir)

	// 创建文件操作工具
	fileTools, err := tools.GetFileTools()
	if err != nil {
		log.Fatal(err)
	}

	// 格式化系统消息
	systemMessages, err := CoderPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	// 创建智能体
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "小码",
		Description: "代码专家，擅长读写和处理代码文件，能够帮助用户完成各种编程任务。",
		Instruction: instruction,
		Model:       common.NewChatModel(),
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: fileTools,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}

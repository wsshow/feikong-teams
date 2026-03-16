package coder

import (
	"context"
	"fkteams/agents/common"

	"github.com/cloudwego/eino/adk"
)

func NewAgent() (adk.Agent, error) {
	return common.NewAgentBuilder("小码", "代码专家，擅长读写和处理代码文件，能够帮助用户完成各种编程任务。").
		WithTemplate(coderPromptTemplate).
		WithTemplateVar("code_dir", common.WorkspaceDir()).
		WithToolNames("file").
		Build(context.Background())
}

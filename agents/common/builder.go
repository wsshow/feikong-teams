package common

import (
	"context"
	"fkteams/agents/middlewares/skills"
	"fkteams/agents/middlewares/summary"
	"fkteams/agents/middlewares/tools/warperror"
	"fkteams/tools"
	"fmt"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// AgentBuilder 智能体构建器，封装公共的创建流程
type AgentBuilder struct {
	name         string
	description  string
	tools        []tool.BaseTool
	toolNames    []string
	template     prompt.ChatTemplate
	templateVars map[string]any

	// 模型配置
	chatModel model.ToolCallingChatModel

	// 中间件
	enableWarperror bool
	enableSummary   bool
	enableSkills    bool
	workspaceDir    string // skills 和 summary 需要
}

// NewAgentBuilder 创建构建器
func NewAgentBuilder(name, description string) *AgentBuilder {
	return &AgentBuilder{
		name:         name,
		description:  description,
		templateVars: map[string]any{"current_time": time.Now().Format("2006-01-02 15:04:05")},
	}
}

// WithTools 设置工具列表
func (b *AgentBuilder) WithTools(tools ...tool.BaseTool) *AgentBuilder {
	b.tools = append(b.tools, tools...)
	return b
}

// WithToolNames 通过工具名称添加工具（在 Build 时通过 tools.GetToolsByName 解析）
func (b *AgentBuilder) WithToolNames(names ...string) *AgentBuilder {
	b.toolNames = append(b.toolNames, names...)
	return b
}

// WithTemplate 设置提示词模板
func (b *AgentBuilder) WithTemplate(t prompt.ChatTemplate) *AgentBuilder {
	b.template = t
	return b
}

// WithTemplateVar 添加模板变量（current_time 已默认设置）
func (b *AgentBuilder) WithTemplateVar(key string, value any) *AgentBuilder {
	b.templateVars[key] = value
	return b
}

// WithModel 使用自定义模型（不设置则使用默认环境变量配置）
func (b *AgentBuilder) WithModel(m model.ToolCallingChatModel) *AgentBuilder {
	b.chatModel = m
	return b
}

// WithFullMiddleware 启用完整中间件栈（warperror + summary + skills）
func (b *AgentBuilder) WithFullMiddleware(workspaceDir string) *AgentBuilder {
	b.enableWarperror = true
	b.enableSummary = true
	b.enableSkills = true
	b.workspaceDir = workspaceDir
	return b
}

// Build 构建智能体
func (b *AgentBuilder) Build(ctx context.Context) (adk.Agent, error) {
	// 模型
	chatModel := b.chatModel
	if chatModel == nil {
		var err error
		chatModel, err = NewChatModel()
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
	}

	// 提示词
	var instruction string
	if b.template != nil {
		msgs, err := b.template.Format(ctx, b.templateVars)
		if err != nil {
			return nil, fmt.Errorf("format prompt: %w", err)
		}
		instruction = msgs[0].Content
	}

	// 通过名称解析工具
	for _, name := range b.toolNames {
		resolved, err := tools.GetToolsByName(name)
		if err != nil {
			return nil, fmt.Errorf("init tool %s: %w", name, err)
		}
		b.tools = append(b.tools, resolved...)
	}

	// 构建配置
	cfg := &adk.ChatModelAgentConfig{
		Name:          b.name,
		Description:   b.description,
		Instruction:   instruction,
		Model:         chatModel,
		MaxIterations: MaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries:  MaxRetries,
			IsRetryAble: IsRetryAble,
		},
	}

	// 工具
	if len(b.tools) > 0 {
		cfg.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: b.tools,
			},
		}
	}

	// 中间件
	if b.enableWarperror || b.enableSummary || b.enableSkills {
		if b.enableWarperror {
			cfg.Middlewares = append(cfg.Middlewares, warperror.NewAgentMiddleware(nil))
		}

		if b.enableSummary {
			summaryMiddleware, err := summary.New(ctx, &summary.Config{
				Model:                      chatModel,
				SystemPrompt:               summary.PromptOfSummary,
				MaxTokensBeforeSummary:     summary.DefaultMaxTokensBeforeSummary,
				MaxTokensForRecentMessages: summary.DefaultMaxTokensForRecentMessages,
			})
			if err != nil {
				return nil, fmt.Errorf("init summary middleware: %w", err)
			}
			cfg.Middlewares = append(cfg.Middlewares, summaryMiddleware)
		}

		if b.enableSkills {
			skillsMiddleware, err := skills.New(ctx, b.workspaceDir)
			if err != nil {
				return nil, fmt.Errorf("init skills middleware: %w", err)
			}
			cfg.Handlers = append(cfg.Handlers, skillsMiddleware)
		}
	}

	return adk.NewChatModelAgent(ctx, cfg)
}

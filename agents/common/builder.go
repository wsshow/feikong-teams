package common

import (
	"context"
	"fkteams/agents/middlewares/dispatch"
	"fkteams/agents/middlewares/skills"
	"fkteams/agents/middlewares/summary"
	"fkteams/agents/middlewares/tools/patch"
	"fkteams/agents/middlewares/tools/warperror"
	"fkteams/agents/retry"
	rootcommon "fkteams/common"
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
	middlewares []adk.AgentMiddleware
	handlers    []adk.ChatModelAgentMiddleware

	// 便捷中间件标记
	enableSummary  bool
	enableSkills   bool
	skillsDir      string
	enableDispatch bool
	dispatchConfig *dispatch.Config
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

// WithMiddleware 添加 AgentMiddleware
func (b *AgentBuilder) WithMiddleware(m ...adk.AgentMiddleware) *AgentBuilder {
	b.middlewares = append(b.middlewares, m...)
	return b
}

// WithHandler 添加 ChatModelAgentMiddleware
func (b *AgentBuilder) WithHandler(h ...adk.ChatModelAgentMiddleware) *AgentBuilder {
	b.handlers = append(b.handlers, h...)
	return b
}

// WithSummary 启用 summary 中间件
func (b *AgentBuilder) WithSummary() *AgentBuilder {
	b.enableSummary = true
	return b
}

// WithSkills 启用 skills 中间件
func (b *AgentBuilder) WithSkills(workspaceDir string) *AgentBuilder {
	b.enableSkills = true
	b.skillsDir = workspaceDir
	return b
}

// WithDispatch 启用子任务分发中间件，cfg 为 nil 时使用默认配置
func (b *AgentBuilder) WithDispatch(cfg *dispatch.Config) *AgentBuilder {
	b.enableDispatch = true
	b.dispatchConfig = cfg
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

	// 用自定义重试包装模型
	retryModel := retry.NewRetryChatModel(chatModel, &retry.ModelRetryConfig{
		MaxRetries:  rootcommon.MaxRetries,
		IsRetryAble: rootcommon.IsRetryAble,
	})

	// 构建配置
	cfg := &adk.ChatModelAgentConfig{
		Name:          b.name,
		Description:   b.description,
		Instruction:   instruction,
		Model:         retryModel,
		MaxIterations: MaxIterations,
	}

	// 工具
	if len(b.tools) > 0 {
		cfg.ToolsConfig = adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: b.tools,
			},
		}
	}

	// 中间件（warperror 默认启用）
	cfg.Middlewares = append(cfg.Middlewares, warperror.NewAgentMiddleware(nil))

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

	cfg.Middlewares = append(cfg.Middlewares, b.middlewares...)

	// patch 中间件默认启用，放在 Handlers 最前面确保其他中间件处理的是完整消息历史
	patchMiddleware, err := patch.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("init patch middleware: %w", err)
	}
	cfg.Handlers = append(cfg.Handlers, patchMiddleware)

	if b.enableSkills {
		skillsMiddleware, err := skills.New(ctx, b.skillsDir)
		if err != nil {
			return nil, fmt.Errorf("init skills middleware: %w", err)
		}
		cfg.Handlers = append(cfg.Handlers, skillsMiddleware)
	}

	if b.enableDispatch {
		if b.dispatchConfig == nil {
			b.dispatchConfig = &dispatch.Config{}
		}
		if b.dispatchConfig.Model == nil {
			b.dispatchConfig.Model = chatModel
		}
		dispatchMiddleware, err := dispatch.New(ctx, b.dispatchConfig)
		if err != nil {
			return nil, fmt.Errorf("init dispatch middleware: %w", err)
		}
		cfg.Handlers = append(cfg.Handlers, dispatchMiddleware)
	}

	cfg.Handlers = append(cfg.Handlers, b.handlers...)

	return adk.NewChatModelAgent(ctx, cfg)
}

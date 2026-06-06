package common

import (
	"context"
	"fkteams/agentcore"
	"fkteams/agentruntime"
	rootcommon "fkteams/common"
	"fkteams/fkenv"
	"fkteams/tools"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// AgentBuilder 智能体构建器，封装公共的创建流程
type AgentBuilder struct {
	name         string
	description  string
	tools        []agentcore.Tool
	toolNames    []string
	instruction  string
	templateVars map[string]any

	// 模型配置
	chatModel agentcore.ChatModel

	// 中间件
	handlers []agentcore.AgentMiddleware

	// 便捷中间件标记
	enableSummary  bool
	enableSkills   bool
	enableDispatch bool
	dispatchConfig *agentcore.DispatchConfig
}

// NewAgentBuilder 创建构建器
func NewAgentBuilder(name, description string) *AgentBuilder {
	return &AgentBuilder{
		name:        name,
		description: description,
		templateVars: map[string]any{
			"os_type":       runtime.GOOS,
			"os_arch":       runtime.GOARCH,
			"workspace_dir": rootcommon.WorkspaceDir(),
		},
	}
}

// WithTools 设置工具列表
func (b *AgentBuilder) WithTools(tools ...agentcore.Tool) *AgentBuilder {
	b.tools = append(b.tools, tools...)
	return b
}

// WithToolNames 通过工具名称添加工具（在 Build 时通过 tools.GetToolsByName 解析）
func (b *AgentBuilder) WithToolNames(names ...string) *AgentBuilder {
	b.toolNames = append(b.toolNames, names...)
	return b
}

func (b *AgentBuilder) WithInstruction(instruction string) *AgentBuilder {
	b.instruction = instruction
	return b
}

// WithTemplateVar 添加模板变量
func (b *AgentBuilder) WithTemplateVar(key string, value any) *AgentBuilder {
	b.templateVars[key] = value
	return b
}

// WithModel 使用自定义模型（不设置则使用默认环境变量配置）
func (b *AgentBuilder) WithModel(m agentcore.ChatModel) *AgentBuilder {
	b.chatModel = m
	return b
}

// WithHandler 添加智能体中间件
func (b *AgentBuilder) WithHandler(h ...agentcore.AgentMiddleware) *AgentBuilder {
	b.handlers = append(b.handlers, h...)
	return b
}

// WithSummary 启用 summary 中间件
func (b *AgentBuilder) WithSummary() *AgentBuilder {
	b.enableSummary = true
	return b
}

// WithSkills 启用 skills 中间件
func (b *AgentBuilder) WithSkills() *AgentBuilder {
	b.enableSkills = true
	return b
}

// WithDispatch 启用子任务分发中间件，cfg 为 nil 时使用默认配置
func (b *AgentBuilder) WithDispatch(cfg *agentcore.DispatchConfig) *AgentBuilder {
	b.enableDispatch = true
	b.dispatchConfig = cfg
	return b
}

// Build 构建智能体
func (b *AgentBuilder) Build(ctx context.Context) (agentcore.Agent, error) {
	// 模型
	coreModel := b.chatModel
	if coreModel == nil {
		var err error
		coreModel, err = NewChatModel()
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
	}
	engine := agentruntime.Engine()
	coreModel, err := engine.DecorateChatModel(ctx, coreModel)
	if err != nil {
		return nil, fmt.Errorf("decorate chat model: %w", err)
	}

	// 提示词
	instruction := b.instruction
	if instruction != "" {
		for key, value := range b.templateVars {
			instruction = strings.ReplaceAll(instruction, "{"+key+"}", fmt.Sprint(value))
		}
	}

	// 通过名称解析工具
	for _, name := range b.toolNames {
		resolved, err := tools.GetToolsByName(name)
		if err != nil {
			return nil, fmt.Errorf("init tool %s: %w", name, err)
		}
		b.tools = append(b.tools, resolved...)
	}

	// 工具元数据分类
	tools.ClassifyTools(b.tools)

	// 构建配置
	cfg := &agentcore.ChatAgentConfig{
		Name:               b.name,
		Description:        b.description,
		Instruction:        instruction,
		Model:              coreModel,
		Tools:              b.tools,
		ToolMiddlewares:    []agentcore.ToolMiddleware{engine.NewDestructiveGuardMiddleware()},
		UnknownToolHandler: unknownToolsHandler,
		ModelRetryConfig:   rootcommon.NewModelRetryConfig(),
		MaxIterations:      MaxIterations(),
		EmitInternalEvents: true,
	}

	// patch 中间件默认启用，放在 Handlers 最前面确保其他中间件处理的是完整消息历史
	patchMiddleware, err := engine.NewPatchMiddleware(ctx)
	if err != nil {
		return nil, fmt.Errorf("init patch middleware: %w", err)
	}
	cfg.Middlewares = append(cfg.Middlewares, patchMiddleware)

	// 中间件（warperror + autocontinue + trimresult 默认启用）
	cfg.Middlewares = append(cfg.Middlewares, engine.NewToolErrorMiddleware())

	acMiddleware, err := engine.NewAutoContinueMiddleware()
	if err != nil {
		return nil, fmt.Errorf("init autocontinue middleware: %w", err)
	}
	cfg.Middlewares = append(cfg.Middlewares, acMiddleware)

	cfg.Middlewares = append(cfg.Middlewares, engine.NewTrimResultMiddleware())

	if b.enableSummary {
		maxTokens := agentcore.DefaultMaxTokensBeforeSummary
		if v := fkenv.Get(fkenv.MaxTokensBeforeSummary); v != "" {
			if n, _ := strconv.Atoi(v); n > 0 {
				maxTokens = n
			}
		}
		summaryMiddleware, err := engine.NewSummaryMiddleware(ctx, &agentcore.SummaryConfig{
			Model:                  coreModel,
			MaxTokensBeforeSummary: maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("init summary middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, summaryMiddleware)
	}

	if b.enableSkills {
		skillsMiddleware, err := engine.NewSkillsMiddleware(ctx)
		if err != nil {
			return nil, fmt.Errorf("init skills middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, skillsMiddleware)
	}

	if b.enableDispatch {
		if b.dispatchConfig == nil {
			b.dispatchConfig = &agentcore.DispatchConfig{}
		}
		if b.dispatchConfig.Model == nil {
			b.dispatchConfig.Model = coreModel
		}
		dispatchMiddleware, err := engine.NewDispatchMiddleware(ctx, b.dispatchConfig)
		if err != nil {
			return nil, fmt.Errorf("init dispatch middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, dispatchMiddleware)
	}

	cfg.Middlewares = append(cfg.Middlewares, b.handlers...)
	return engine.NewChatModelAgent(ctx, cfg)
}

// unknownToolsHandler 处理模型幻觉出的不存在的工具调用，
// 将错误包装为字符串结果返回给模型而非中断执行。
func unknownToolsHandler(_ context.Context, name, _ string) (string, error) {
	return fmt.Sprintf("Tool '%s' does not exist. Please check the available tools and try again.", name), nil
}

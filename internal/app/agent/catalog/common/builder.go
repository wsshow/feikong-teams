package common

import (
	"context"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/env"
	runtimeregistry "fkteams/internal/runtime/registry"
	"fkteams/internal/runtime/resources"
	"fkteams/internal/runtime/retry"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// AgentBuilder 智能体构建器，封装公共的创建流程
type AgentBuilder struct {
	name         string
	description  string
	tools        []runtimeport.Tool
	toolNames    []string
	instruction  string
	templateVars map[string]any

	// 模型配置
	chatModel runtimeport.ChatModel

	// 中间件
	handlers []runtimeport.AgentMiddleware

	// 便捷中间件标记
	enableSummary  bool
	enableSkills   bool
	enableDispatch bool
	dispatchConfig *runtimeport.DispatchConfig
}

// NewAgentBuilder 创建构建器
func NewAgentBuilder(name, description string) *AgentBuilder {
	return &AgentBuilder{
		name:        name,
		description: description,
		templateVars: map[string]any{
			"os_type":       runtime.GOOS,
			"os_arch":       runtime.GOARCH,
			"workspace_dir": appdata.WorkspaceDir(),
		},
	}
}

// WithTools 设置工具列表
func (b *AgentBuilder) WithTools(tools ...runtimeport.Tool) *AgentBuilder {
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
func (b *AgentBuilder) WithModel(m runtimeport.ChatModel) *AgentBuilder {
	b.chatModel = m
	return b
}

// WithHandler 添加智能体中间件
func (b *AgentBuilder) WithHandler(h ...runtimeport.AgentMiddleware) *AgentBuilder {
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
func (b *AgentBuilder) WithDispatch(cfg *runtimeport.DispatchConfig) *AgentBuilder {
	b.enableDispatch = true
	b.dispatchConfig = cfg
	return b
}

// Build 构建智能体
func (b *AgentBuilder) Build(ctx context.Context) (runtimeport.Agent, error) {
	// 模型
	coreModel := b.chatModel
	if coreModel == nil {
		var err error
		coreModel, err = NewChatModel()
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
	}
	engine := runtimeregistry.Engine()
	coreModel, err := decorateChatModel(ctx, engine, coreModel)
	if err != nil {
		return nil, fmt.Errorf("decorate chat model: %w", err)
	}
	agentPipelineProvider, hasAgentPipelineProvider := engine.(runtimeport.AgentPipelineProvider)

	// 提示词
	instruction := b.instruction
	if instruction != "" {
		for key, value := range b.templateVars {
			instruction = strings.ReplaceAll(instruction, "{"+key+"}", fmt.Sprint(value))
		}
	}

	// 通过名称解析工具
	toolList := append([]runtimeport.Tool(nil), b.tools...)
	var cleaner *resources.Cleaner
	if state := appstate.FromContext(ctx); state != nil {
		cleaner = state.Cleaner()
	}
	builtinTools, err := tools.GetBuiltinCapabilityToolsWithCleaner(cleaner)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, builtinTools...)

	seenToolNames := make(map[string]bool, len(b.toolNames))
	for _, name := range b.toolNames {
		if seenToolNames[name] {
			continue
		}
		seenToolNames[name] = true
		resolved, err := tools.GetToolsByNameWithCleaner(name, cleaner)
		if err != nil {
			return nil, fmt.Errorf("init tool %s: %w", name, err)
		}
		if !strings.HasPrefix(name, "mcp-") {
			if err := tools.MarkPolicyRequired(resolved); err != nil {
				return nil, fmt.Errorf("mark tool policy %s: %w", name, err)
			}
		}
		toolList = append(toolList, resolved...)
	}

	// 工具元数据分类
	if err := tools.ClassifyTools(toolList); err != nil {
		return nil, fmt.Errorf("classify tools: %w", err)
	}

	// 构建配置
	cfg := &runtimeport.ChatAgentConfig{
		Name:               b.name,
		Description:        b.description,
		Instruction:        instruction,
		Model:              coreModel,
		Tools:              toolList,
		ToolMiddlewares:    defaultToolMiddlewares(engine),
		UnknownToolHandler: unknownToolsHandler,
		ModelRetryConfig:   retry.NewModelRetryConfig(),
		MaxIterations:      MaxIterations(),
		EmitInternalEvents: true,
	}

	if hasAgentPipelineProvider {
		defaultMiddlewares, err := agentPipelineProvider.DefaultAgentMiddlewares(ctx)
		if err != nil {
			return nil, fmt.Errorf("init default agent middlewares: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, defaultMiddlewares...)
	}

	if b.enableSummary {
		if !hasAgentPipelineProvider {
			return nil, fmt.Errorf("runtime does not support summary middleware")
		}
		maxTokens := runtimeport.DefaultMaxTokensBeforeSummary
		if v := env.Get(env.MaxTokensBeforeSummary); v != "" {
			if n, _ := strconv.Atoi(v); n > 0 {
				maxTokens = n
			}
		}
		summaryMiddleware, err := agentPipelineProvider.NewSummaryMiddleware(ctx, &runtimeport.SummaryConfig{
			Model:                  coreModel,
			MaxTokensBeforeSummary: maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("init summary middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, summaryMiddleware)
	}

	if b.enableSkills {
		if !hasAgentPipelineProvider {
			return nil, fmt.Errorf("runtime does not support skills middleware")
		}
		skillsMiddleware, err := agentPipelineProvider.NewSkillsMiddleware(ctx)
		if err != nil {
			return nil, fmt.Errorf("init skills middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, skillsMiddleware)
	}

	if b.enableDispatch {
		dispatchConfig := b.dispatchConfig
		if dispatchConfig == nil {
			dispatchConfig = &runtimeport.DispatchConfig{}
		} else {
			copied := *dispatchConfig
			dispatchConfig = &copied
		}
		if dispatchConfig.Model == nil {
			dispatchConfig.Model = coreModel
		}
		if !hasAgentPipelineProvider {
			return nil, fmt.Errorf("runtime does not support dispatch middleware")
		}
		dispatchMiddleware, err := agentPipelineProvider.NewDispatchMiddleware(ctx, dispatchConfig)
		if err != nil {
			return nil, fmt.Errorf("init dispatch middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, dispatchMiddleware)
	}

	if hasAgentPipelineProvider {
		agentsMDMiddleware, err := agentPipelineProvider.NewAgentsMDMiddleware(ctx)
		if err != nil {
			return nil, fmt.Errorf("init agents.md middleware: %w", err)
		}
		cfg.Middlewares = append(cfg.Middlewares, agentsMDMiddleware)
	}

	cfg.Middlewares = append(cfg.Middlewares, b.handlers...)
	return engine.NewChatModelAgent(ctx, cfg)
}

// unknownToolsHandler 处理模型幻觉出的不存在的工具调用，
// 将错误包装为字符串结果返回给模型而非中断执行。
func unknownToolsHandler(_ context.Context, name, _ string) (string, error) {
	return fmt.Sprintf("Tool '%s' does not exist. Please check the available tools and try again.", name), nil
}

func decorateChatModel(ctx context.Context, engine runtimeport.Engine, model runtimeport.ChatModel) (runtimeport.ChatModel, error) {
	decorator, ok := engine.(runtimeport.ModelDecorator)
	if !ok {
		return model, nil
	}
	return decorator.DecorateChatModel(ctx, model)
}

func defaultToolMiddlewares(engine runtimeport.Engine) []runtimeport.ToolMiddleware {
	provider, ok := engine.(runtimeport.ToolPipelineProvider)
	if !ok {
		return nil
	}
	return provider.DefaultToolMiddlewares()
}

package common

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"fkteams/internal/app/appstate"
	"fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/env"
	"fkteams/internal/runtime/resources"
	"fkteams/internal/runtime/retry"
	"fkteams/internal/runtime/toolpolicy"
)

type Profile string

const (
	ProfileBare      Profile = "bare"
	ProfileWorkspace Profile = "workspace"
	ProfileFull      Profile = "full"
	ProfileTeam      Profile = "team"
)

type Definition struct {
	Name         string
	Description  string
	Instruction  string
	TemplateVars map[string]any
	Profile      Profile

	Model     runtimeport.ChatModel
	Tools     []runtimeport.Tool
	ToolNames []string

	Handlers       []runtimeport.AgentMiddleware
	EnableSummary  bool
	EnableSkills   bool
	DispatchConfig *runtimeport.DispatchConfig
}

type ResolvedAgent struct {
	Definition
	InstructionText string
	Model           runtimeport.ChatModel
	Tools           []runtimeport.Tool
	Middlewares     []runtimeport.AgentMiddleware
	ToolMiddleware  []runtimeport.ToolMiddleware
}

type Resolver struct{}

type Assembler struct{}

func NewDefinition(name, description string) Definition {
	return Definition{
		Name:        name,
		Description: description,
		Profile:     ProfileWorkspace,
		TemplateVars: map[string]any{
			"os_type": runtime.GOOS,
			"os_arch": runtime.GOARCH,
		},
	}
}

func BuildAgent(ctx context.Context, def Definition) (runtimeport.Agent, error) {
	resolved, err := (Resolver{}).Resolve(ctx, def)
	if err != nil {
		return nil, err
	}
	return (Assembler{}).Build(ctx, resolved)
}

func (r Resolver) Resolve(ctx context.Context, def Definition) (*ResolvedAgent, error) {
	if def.Name == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if def.Profile == "" {
		def.Profile = ProfileWorkspace
	}

	coreModel := def.Model
	if coreModel == nil {
		var err error
		coreModel, err = NewChatModel(ctx)
		if err != nil {
			return nil, fmt.Errorf("create chat model: %w", err)
		}
	}

	pipelineRuntime, hasPipelineRuntime := runtimeport.PipelineRuntimeFromContext(ctx)
	coreModel, err := decorateChatModel(ctx, pipelineRuntime, coreModel)
	if err != nil {
		return nil, fmt.Errorf("decorate chat model: %w", err)
	}

	instruction := renderInstruction(def.Instruction, def.TemplateVars)
	cleaner := cleanerFromContext(ctx)

	toolList, err := r.resolveTools(ctx, def, cleaner)
	if err != nil {
		return nil, err
	}
	if err := toolpolicy.ClassifyTools(toolList); err != nil {
		return nil, fmt.Errorf("classify tools: %w", err)
	}

	middlewares, err := r.resolveMiddlewares(ctx, def, coreModel, pipelineRuntime, hasPipelineRuntime, cleaner)
	if err != nil {
		return nil, err
	}

	return &ResolvedAgent{
		Definition:      def,
		InstructionText: instruction,
		Model:           coreModel,
		Tools:           toolList,
		Middlewares:     append(middlewares, def.Handlers...),
		ToolMiddleware:  defaultToolMiddlewares(pipelineRuntime, hasPipelineRuntime),
	}, nil
}

func (r Resolver) resolveTools(ctx context.Context, def Definition, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	toolList := append([]runtimeport.Tool(nil), def.Tools...)
	if profileIncludesWorkspace(def.Profile) {
		builtinTools, err := tools.GetBuiltinCapabilityToolsWithCleaner(cleaner)
		if err != nil {
			return nil, err
		}
		toolList = append(toolList, builtinTools...)
	}

	namedTools, err := resolveNamedToolGroups(ctx, def.ToolNames, cleaner)
	if err != nil {
		return nil, err
	}
	toolList = append(toolList, namedTools...)
	return toolList, nil
}

func (r Resolver) resolveMiddlewares(ctx context.Context, def Definition, model runtimeport.ChatModel, pipelineRuntime runtimeport.PipelineRuntime, hasPipelineRuntime bool, cleaner *resources.Cleaner) ([]runtimeport.AgentMiddleware, error) {
	if !hasPipelineRuntime {
		if profileIncludesWorkspace(def.Profile) || def.EnableSummary || def.EnableSkills || def.DispatchConfig != nil {
			return nil, fmt.Errorf("runtime does not support agent middlewares")
		}
		return nil, nil
	}

	var middlewares []runtimeport.AgentMiddleware
	if profileIncludesWorkspace(def.Profile) {
		defaultMiddlewares, err := pipelineRuntime.DefaultAgentMiddlewares(ctx)
		if err != nil {
			return nil, fmt.Errorf("init default agent middlewares: %w", err)
		}
		middlewares = append(middlewares, defaultMiddlewares...)
	}

	enableSummary := def.EnableSummary || def.Profile == ProfileFull || def.Profile == ProfileTeam
	if enableSummary {
		summaryMiddleware, err := pipelineRuntime.NewSummaryMiddleware(ctx, &runtimeport.SummaryConfig{
			Model:                  model,
			MaxTokensBeforeSummary: maxTokensBeforeSummary(),
		})
		if err != nil {
			return nil, fmt.Errorf("init summary middleware: %w", err)
		}
		middlewares = append(middlewares, summaryMiddleware)
	}

	enableSkills := def.EnableSkills || def.Profile == ProfileFull || def.Profile == ProfileTeam
	if enableSkills {
		skillsMiddleware, err := pipelineRuntime.NewSkillsMiddleware(ctx)
		if err != nil {
			return nil, fmt.Errorf("init skills middleware: %w", err)
		}
		middlewares = append(middlewares, skillsMiddleware)
	}

	if def.DispatchConfig != nil {
		dispatchConfig := *def.DispatchConfig
		if dispatchConfig.Model == nil {
			dispatchConfig.Model = model
		}
		dispatchTools, err := resolveNamedToolGroups(ctx, dispatchConfig.ToolNames, cleaner)
		if err != nil {
			return nil, fmt.Errorf("init dispatch tools: %w", err)
		}
		dispatchConfig.ToolNames = nil
		dispatchConfig.Tools = append(dispatchConfig.Tools, dispatchTools...)
		if err := toolpolicy.ClassifyTools(dispatchConfig.Tools); err != nil {
			return nil, fmt.Errorf("classify dispatch tools: %w", err)
		}
		dispatchMiddleware, err := pipelineRuntime.NewDispatchMiddleware(ctx, &dispatchConfig)
		if err != nil {
			return nil, fmt.Errorf("init dispatch middleware: %w", err)
		}
		middlewares = append(middlewares, dispatchMiddleware)
	}

	if profileIncludesWorkspace(def.Profile) {
		agentsMDMiddleware, err := pipelineRuntime.NewAgentsMDMiddleware(ctx)
		if err != nil {
			return nil, fmt.Errorf("init agents.md middleware: %w", err)
		}
		middlewares = append(middlewares, agentsMDMiddleware)
	}
	return middlewares, nil
}

func (a Assembler) Build(ctx context.Context, resolved *ResolvedAgent) (runtimeport.Agent, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved agent is nil")
	}
	agentRuntime, err := runtimeport.RequireAgentRuntime(ctx)
	if err != nil {
		return nil, err
	}
	return agentRuntime.NewChatModelAgent(ctx, &runtimeport.ChatAgentConfig{
		Name:               resolved.Name,
		Description:        resolved.Description,
		Instruction:        resolved.InstructionText,
		Model:              resolved.Model,
		Tools:              resolved.Tools,
		ToolMiddlewares:    resolved.ToolMiddleware,
		UnknownToolHandler: unknownToolsHandler,
		ModelRetryConfig:   retry.NewModelRetryConfig(),
		MaxIterations:      MaxIterations(),
		EmitInternalEvents: true,
		Middlewares:        resolved.Middlewares,
	})
}

func profileIncludesWorkspace(profile Profile) bool {
	return profile == ProfileWorkspace || profile == ProfileFull || profile == ProfileTeam
}

func renderInstruction(instruction string, vars map[string]any) string {
	if instruction == "" {
		return ""
	}
	for key, value := range vars {
		instruction = strings.ReplaceAll(instruction, "{"+key+"}", fmt.Sprint(value))
	}
	return instruction
}

func cleanerFromContext(ctx context.Context) *resources.Cleaner {
	if state := appstate.FromContext(ctx); state != nil {
		return state.Cleaner()
	}
	return nil
}

func maxTokensBeforeSummary() int {
	maxTokens := runtimeport.DefaultMaxTokensBeforeSummary
	if v := env.Get(env.MaxTokensBeforeSummary); v != "" {
		if n, _ := strconv.Atoi(v); n > 0 {
			maxTokens = n
		}
	}
	return maxTokens
}

func resolveNamedToolGroups(ctx context.Context, names []string, cleaner *resources.Cleaner) ([]runtimeport.Tool, error) {
	seen := make(map[string]bool, len(names))
	var result []runtimeport.Tool
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		resolved, err := tools.GetToolsByNameWithCleaner(ctx, name, cleaner)
		if err != nil {
			return nil, fmt.Errorf("init tool %s: %w", name, err)
		}
		if !strings.HasPrefix(name, "mcp-") {
			if err := toolpolicy.MarkPolicyRequired(resolved); err != nil {
				return nil, fmt.Errorf("mark tool policy %s: %w", name, err)
			}
		}
		result = append(result, resolved...)
	}
	return result, nil
}

// unknownToolsHandler 处理模型幻觉出的不存在的工具调用，
// 将错误包装为字符串结果返回给模型而非中断执行。
func unknownToolsHandler(_ context.Context, name, _ string) (string, error) {
	return fmt.Sprintf("Tool '%s' does not exist. Please check the available tools and try again.", name), nil
}

func decorateChatModel(ctx context.Context, runtime runtimeport.PipelineRuntime, model runtimeport.ChatModel) (runtimeport.ChatModel, error) {
	if runtime == nil {
		return model, nil
	}
	decorator, ok := runtime.(runtimeport.ModelDecorator)
	if !ok {
		return model, nil
	}
	return decorator.DecorateChatModel(ctx, model)
}

func defaultToolMiddlewares(runtime runtimeport.PipelineRuntime, ok bool) []runtimeport.ToolMiddleware {
	if !ok {
		return nil
	}
	return runtime.DefaultToolMiddlewares()
}

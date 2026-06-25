package eino

import (
	"context"
	"fkteams/agentcore"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
)

func NewChatModelAgent(ctx context.Context, cfg *agentcore.ChatAgentConfig) (agentcore.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("agent config is nil")
	}
	chatModel, err := AdaptChatModelForRunner(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("adapt chat model: %w", err)
	}
	runnerTools, err := AdaptToolsForRunner(ctx, cfg.Tools)
	if err != nil {
		return nil, fmt.Errorf("adapt tools: %w", err)
	}
	runnerHandlers, err := AdaptAgentMiddlewaresForRunner(cfg.Middlewares)
	if err != nil {
		return nil, fmt.Errorf("adapt middleware: %w", err)
	}
	runnerToolMiddlewares, err := adaptToolMiddlewaresForRunner(cfg.ToolMiddlewares)
	if err != nil {
		return nil, fmt.Errorf("adapt tool middleware: %w", err)
	}

	agentCfg := &adk.ChatModelAgentConfig{
		Name:             cfg.Name,
		Description:      cfg.Description,
		Instruction:      cfg.Instruction,
		Model:            chatModel,
		ModelRetryConfig: AdaptModelRetryConfigForRunner(cfg.ModelRetryConfig),
		MaxIterations:    cfg.MaxIterations,
		Handlers:         runnerHandlers,
		ToolsConfig: adk.ToolsConfig{
			EmitInternalEvents: cfg.EmitInternalEvents,
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               runnerTools,
				UnknownToolsHandler: adaptUnknownToolHandlerForRunner(cfg.Name, cfg.UnknownToolHandler),
				ToolCallMiddlewares: runnerToolMiddlewares,
			},
		},
	}

	agent, err := adk.NewChatModelAgent(ctx, agentCfg)
	if err != nil {
		return nil, err
	}
	return WrapNamedAgent(cfg.Name, cfg.Description, agent), nil
}

func NewLoopAgent(ctx context.Context, cfg *agentcore.LoopAgentConfig) (agentcore.Agent, error) {
	if cfg == nil {
		return nil, fmt.Errorf("loop agent config is nil")
	}
	subAgents, err := AdaptAgentsForRunner(cfg.SubAgents)
	if err != nil {
		return nil, err
	}
	loopAgent, err := adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          cfg.Name,
		Description:   cfg.Description,
		SubAgents:     subAgents,
		MaxIterations: cfg.MaxIterations,
	})
	if err != nil {
		return nil, err
	}
	return WrapNamedAgent(cfg.Name, cfg.Description, loopAgent), nil
}

func NewRunnerFromConfig(ctx context.Context, cfg agentcore.RunnerConfig) (agentcore.Runner, error) {
	runnerAgent, err := AdaptAgentForRunner(cfg.Agent)
	if err != nil {
		return nil, err
	}
	return NewRunner(adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           runnerAgent,
		EnableStreaming: cfg.EnableStreaming,
		CheckPointStore: adaptCheckPointStoreForRunner(cfg.CheckPointStore),
	})), nil
}

func adaptToolMiddlewaresForRunner(middlewares []agentcore.ToolMiddleware) ([]compose.ToolMiddleware, error) {
	result := make([]compose.ToolMiddleware, 0, len(middlewares))
	for _, middleware := range middlewares {
		if middleware == nil {
			continue
		}
		runnerMiddleware, err := AdaptToolMiddlewareForRunner(middleware)
		if err != nil {
			return nil, err
		}
		result = append(result, runnerMiddleware)
	}
	return result, nil
}

func adaptUnknownToolHandlerForRunner(agentName string, handler agentcore.UnknownToolHandler) func(context.Context, string, string) (string, error) {
	if handler == nil {
		return nil
	}
	return func(ctx context.Context, name, arguments string) (string, error) {
		result, err := handler(ctx, name, arguments)
		if err == nil {
			recordUnknownToolResult(ctx, unknownToolReport{
				AgentName:  agentName,
				ToolName:   name,
				ToolArgs:   arguments,
				ToolResult: result,
			})
		}
		return result, err
	}
}

func AdaptModelRetryConfigForRunner(cfg *agentcore.ModelRetryConfig) *adk.ModelRetryConfig {
	if cfg == nil {
		return nil
	}
	return &adk.ModelRetryConfig{
		MaxRetries: cfg.MaxRetries,
		ShouldRetry: func(ctx context.Context, retryCtx *adk.RetryContext) *adk.RetryDecision {
			var coreRetryCtx *agentcore.RetryContext
			if retryCtx != nil {
				coreRetryCtx = &agentcore.RetryContext{Err: retryCtx.Err}
			}
			decision := cfg.ShouldRetry(ctx, coreRetryCtx)
			if decision == nil {
				return nil
			}
			return &adk.RetryDecision{
				Retry:        decision.Retry,
				RejectReason: decision.RejectReason,
			}
		},
	}
}

type checkPointStoreAdapter struct {
	inner agentcore.CheckPointStore
}

func adaptCheckPointStoreForRunner(store agentcore.CheckPointStore) compose.CheckPointStore {
	if store == nil {
		return nil
	}
	return &checkPointStoreAdapter{inner: store}
}

func (s *checkPointStoreAdapter) Set(ctx context.Context, key string, value []byte) error {
	return s.inner.Set(ctx, key, value)
}

func (s *checkPointStoreAdapter) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return s.inner.Get(ctx, key)
}

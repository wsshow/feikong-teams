package agentcore

import (
	"context"
	"time"
)

type Agent interface {
	Name() string
	Description() string
}

type AgentToolNameFunc func(displayName string, index int) string

type AgentToolDisplayFunc func(toolName, displayName string)

type AgentToolConfig struct {
	ToolName        AgentToolNameFunc
	RegisterDisplay AgentToolDisplayFunc
}

type UnknownToolHandler func(ctx context.Context, name, arguments string) (string, error)

type RetryContext struct {
	Err error
}

type RetryDecision struct {
	Retry        bool
	RejectReason string
}

type ModelRetryConfig struct {
	MaxRetries  int
	ShouldRetry func(ctx context.Context, retryCtx *RetryContext) *RetryDecision
}

type ChatAgentConfig struct {
	Name               string
	Description        string
	Instruction        string
	Model              ChatModel
	Tools              []Tool
	ToolMiddlewares    []ToolMiddleware
	UnknownToolHandler UnknownToolHandler
	Middlewares        []AgentMiddleware
	ModelRetryConfig   *ModelRetryConfig
	MaxIterations      int
	EmitInternalEvents bool
}

type LoopAgentConfig struct {
	Name          string
	Description   string
	SubAgents     []Agent
	MaxIterations int
}

type DeepAgentConfig struct {
	Name               string
	Description        string
	Model              ChatModel
	Tools              []Tool
	SubAgents          []Agent
	Middlewares        []AgentMiddleware
	ModelRetryConfig   *ModelRetryConfig
	MaxIterations      int
	EmitInternalEvents bool
}

type RunnerConfig struct {
	Agent           Agent
	EnableStreaming bool
	CheckPointStore CheckPointStore
}

type SummaryPersistCallback func(summaryText string)

const DefaultMaxTokensBeforeSummary = 800 * 1024

type summaryPersistCallbackKey struct{}

func WithSummaryPersistCallback(ctx context.Context, cb SummaryPersistCallback) context.Context {
	return context.WithValue(ctx, summaryPersistCallbackKey{}, cb)
}

func SummaryPersistCallbackFromContext(ctx context.Context) (SummaryPersistCallback, bool) {
	cb, ok := ctx.Value(summaryPersistCallbackKey{}).(SummaryPersistCallback)
	return cb, ok
}

type SummaryConfig struct {
	MaxTokensBeforeSummary int
	Model                  ChatModel
}

type DispatchConfig struct {
	Model          ChatModel
	ToolNames      []string
	Tools          []Tool
	MaxConcurrency int
	TaskTimeout    time.Duration
}

type Engine interface {
	NewChatModelAgent(ctx context.Context, cfg *ChatAgentConfig) (Agent, error)
	NewLoopAgent(ctx context.Context, cfg *LoopAgentConfig) (Agent, error)
	NewDeepAgent(ctx context.Context, cfg *DeepAgentConfig) (Agent, error)
	NewRunner(ctx context.Context, cfg RunnerConfig) (Runner, error)
	NewAgentTools(ctx context.Context, subAgents []Agent, cfg AgentToolConfig) ([]Tool, error)
}

type ModelDecorator interface {
	DecorateChatModel(ctx context.Context, model ChatModel) (ChatModel, error)
}

type AgentPipelineProvider interface {
	DefaultAgentMiddlewares(ctx context.Context) ([]AgentMiddleware, error)
	NewSteeringMiddleware() AgentMiddleware
	NewSummaryMiddleware(ctx context.Context, cfg *SummaryConfig) (AgentMiddleware, error)
	NewSkillsMiddleware(ctx context.Context) (AgentMiddleware, error)
	NewDispatchMiddleware(ctx context.Context, cfg *DispatchConfig) (AgentMiddleware, error)
}

type ToolPipelineProvider interface {
	DefaultToolMiddlewares() []ToolMiddleware
}

type MCPToolProvider interface {
	MCPTools(ctx context.Context, rawClient any) ([]Tool, error)
}

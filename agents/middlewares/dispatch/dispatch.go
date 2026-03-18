// Package dispatch 提供子任务并行分发中间件，向父智能体注入 dispatch_tasks 工具，
// 将独立子任务下发到子智能体并行执行。
package dispatch

import (
	"context"
	"fmt"
	"time"

	"fkteams/tools"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultMaxConcurrency = 3
	defaultTaskTimeout    = 5 * time.Minute
	defaultMaxIterations  = 60
	defaultMaxRetries     = 3
)

// Config 分发中间件配置。未指定工具时子智能体自动继承父智能体的工具。
type Config struct {
	Model          model.ToolCallingChatModel // 子智能体模型（由 AgentBuilder 自动填充）
	ToolNames      []string                   // 工具名称，通过 tools.GetToolsByName 解析
	Tools          []tool.BaseTool            // 工具实例，与 ToolNames 合并
	MaxConcurrency int                        // 最大并发数（默认 3）
	TaskTimeout    time.Duration              // 单任务超时（默认 5min）
}

func (c *Config) defaults() {
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = defaultMaxConcurrency
	}
	if c.TaskTimeout <= 0 {
		c.TaskTimeout = defaultTaskTimeout
	}
}

// New 创建分发中间件
func New(_ context.Context, cfg *Config) (adk.ChatModelAgentMiddleware, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("dispatch: Model is required")
	}
	cfg.defaults()

	var resolved []tool.BaseTool
	for _, name := range cfg.ToolNames {
		t, err := tools.GetToolsByName(name)
		if err != nil {
			return nil, fmt.Errorf("dispatch: init tool %s: %w", name, err)
		}
		resolved = append(resolved, t...)
	}

	return &middleware{
		chatModel:      cfg.Model,
		tools:          append(resolved, cfg.Tools...),
		maxConcurrency: int64(cfg.MaxConcurrency),
		taskTimeout:    cfg.TaskTimeout,
	}, nil
}

type middleware struct {
	*adk.BaseChatModelAgentMiddleware
	chatModel      model.ToolCallingChatModel
	tools          []tool.BaseTool
	maxConcurrency int64
	taskTimeout    time.Duration
}

// BeforeAgent 注入 dispatch_tasks 工具和提示词，未配置工具时继承父智能体的工具
func (m *middleware) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	if len(m.tools) == 0 {
		m.tools = make([]tool.BaseTool, len(runCtx.Tools))
		copy(m.tools, runCtx.Tools)
	}

	t, err := utils.InferTool("dispatch_tasks", toolDesc, m.executeTasks)
	if err != nil {
		return ctx, runCtx, fmt.Errorf("dispatch: create tool: %w", err)
	}
	runCtx.Tools = append(runCtx.Tools, t)
	runCtx.Instruction += dispatchPrompt
	return ctx, runCtx, nil
}

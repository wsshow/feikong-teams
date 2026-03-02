package warperror

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ErrorHandler 定义工具调用错误处理函数
type ErrorHandler func(ctx context.Context, in *compose.ToolInput, err error) string

// defaultErrorHandler 默认错误处理：将错误信息格式化为工具调用结果返回给 LLM
func defaultErrorHandler(ctx context.Context, in *compose.ToolInput, err error) string {
	return fmt.Sprintf("Failed to call tool '%s', error message: '%s'", in.Name, err.Error())
}

// Config 中间件配置
type Config struct {
	// Handler 自定义错误处理函数，为空时使用默认处理
	Handler ErrorHandler
}

// NewAgentMiddleware 创建 ADK AgentMiddleware，通过 WrapToolCall 拦截工具调用错误
// 将错误转换为成功的工具输出返回给 LLM，避免中断 Agent 流程
func NewAgentMiddleware(cfg *Config) adk.AgentMiddleware {
	return adk.AgentMiddleware{
		WrapToolCall: New(cfg),
	}
}

// New 创建工具错误处理中间件
// 拦截工具调用错误，将其转换为成功的工具输出返回给 LLM，避免中断 Agent 流程
func New(cfg *Config) compose.ToolMiddleware {
	handler := defaultErrorHandler
	if cfg != nil && cfg.Handler != nil {
		handler = cfg.Handler
	}

	return compose.ToolMiddleware{
		Invokable:  newInvokable(handler),
		Streamable: newStreamable(handler),
	}
}

// newInvokable 创建非流式工具调用中间件
func newInvokable(handler ErrorHandler) compose.InvokableToolMiddleware {
	return func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
		return func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
			output, err := next(ctx, in)
			if err != nil {
				// 中断重运行错误需要透传
				if _, ok := compose.IsInterruptRerunError(err); ok {
					return nil, err
				}
				result := handler(ctx, in, err)
				return &compose.ToolOutput{Result: result}, nil
			}
			return output, nil
		}
	}
}

// newStreamable 创建流式工具调用中间件
func newStreamable(handler ErrorHandler) compose.StreamableToolMiddleware {
	return func(next compose.StreamableToolEndpoint) compose.StreamableToolEndpoint {
		return func(ctx context.Context, in *compose.ToolInput) (*compose.StreamToolOutput, error) {
			streamOutput, err := next(ctx, in)
			if err != nil {
				// 中断重运行错误需要透传
				if _, ok := compose.IsInterruptRerunError(err); ok {
					return nil, err
				}
				result := handler(ctx, in, err)
				return &compose.StreamToolOutput{
					Result: schema.StreamReaderFromArray([]string{result}),
				}, nil
			}
			return streamOutput, nil
		}
	}
}

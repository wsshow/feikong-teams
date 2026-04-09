// Package common provides shared utilities for agents.
package common

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// errorSafeAgent 包装子智能体，捕获其运行时的错误（如网络中断、API 超时、超出迭代上限等），
// 将其转换为文本消息返回，而非让错误穿透终止整个对话。
type errorSafeAgent struct {
	inner adk.Agent
}

// WrapErrorSafe 将智能体包装为错误安全版本。
func WrapErrorSafe(agent adk.Agent) adk.Agent {
	return &errorSafeAgent{inner: agent}
}

func (e *errorSafeAgent) Name(ctx context.Context) string {
	return e.inner.Name(ctx)
}

func (e *errorSafeAgent) Description(ctx context.Context) string {
	return e.inner.Description(ctx)
}

func (e *errorSafeAgent) Run(ctx context.Context, input *adk.AgentInput, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	innerIter := e.inner.Run(ctx, input, opts...)

	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	go func() {
		defer gen.Close()

		for {
			event, ok := innerIter.Next()
			if !ok {
				break
			}

			if event.Err != nil {
				errMsg := fmt.Sprintf("[执行出错] %s 执行任务时遇到错误: %s。任务未完成。",
					e.inner.Name(ctx), event.Err.Error())

				gen.Send(&adk.AgentEvent{
					AgentName: event.AgentName,
					RunPath:   event.RunPath,
					Output: &adk.AgentOutput{
						MessageOutput: &adk.MessageVariant{
							Message: schema.AssistantMessage(errMsg, nil),
							Role:    schema.Assistant,
						},
					},
				})
				break
			}

			gen.Send(event)
		}
	}()

	return iter
}

package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// errorSafeAgent 包装子智能体，捕获其运行时错误并转换为文本消息。
type errorSafeAgent struct {
	inner adk.Agent
}

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
	return e.wrap(ctx, innerIter)
}

func (e *errorSafeAgent) Resume(ctx context.Context, info *adk.ResumeInfo, opts ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	inner, ok := e.inner.(adk.ResumableAgent)
	if !ok {
		return newErrorAgentIterator(fmt.Errorf("agent %q does not support resume", e.inner.Name(ctx)))
	}
	innerIter := inner.Resume(ctx, info, opts...)
	return e.wrap(ctx, innerIter)
}

func (e *errorSafeAgent) wrap(ctx context.Context, innerIter *adk.AsyncIterator[*adk.AgentEvent]) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()

	go func() {
		defer gen.Close()
		collector := newPartialOutputCollector()

		for {
			event, ok := innerIter.Next()
			if !ok {
				break
			}

			if event.Err != nil {
				errMsg := formatAgentError(e.inner.Name(ctx), event.Err, collector.String())

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

			collector.Observe(event)
			gen.Send(event)
		}
	}()

	return iter
}

func newErrorAgentIterator(err error) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(&adk.AgentEvent{Err: err})
	gen.Close()
	return iter
}

type partialOutputCollector struct {
	mu sync.Mutex
	sb strings.Builder
}

func newPartialOutputCollector() *partialOutputCollector {
	return &partialOutputCollector{}
}

func (c *partialOutputCollector) Observe(event *adk.AgentEvent) {
	if event == nil || event.Output == nil || event.Output.MessageOutput == nil {
		return
	}
	msgOutput := event.Output.MessageOutput
	if msgOutput.Message != nil && msgOutput.Message.Content != "" {
		c.Append(msgOutput.Message.Content)
		return
	}
	if msgOutput.MessageStream == nil {
		return
	}

	streams := msgOutput.MessageStream.Copy(2)
	msgOutput.MessageStream = streams[0]
	go c.consumeStream(streams[1])
}

func (c *partialOutputCollector) consumeStream(stream *schema.StreamReader[*schema.Message]) {
	defer stream.Close()
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			return
		}
		if chunk != nil && chunk.Content != "" {
			c.Append(chunk.Content)
		}
	}
}

func (c *partialOutputCollector) Append(content string) {
	if content == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sb.WriteString(content)
}

func (c *partialOutputCollector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.sb.String())
}

func formatAgentError(agentName string, err error, partial string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[执行出错] %s 执行任务时遇到错误，任务未完整完成。\n\n", agentName)
	if partial != "" {
		fmt.Fprintf(&sb, "## 已产生的可用内容\n%s\n\n", partial)
	}
	fmt.Fprintf(&sb, "## 错误信息\n%s\n\n", err.Error())
	sb.WriteString("## 给 coordinator 的处理建议\n")
	sb.WriteString("- 不要丢弃“已产生的可用内容”，先判断哪些信息仍可用于回答。\n")
	sb.WriteString("- 分析错误类型；如果是内容风控、限流或临时网络错误，调整任务表述、缩小范围或换用其他成员/工具补齐缺口。\n")
	sb.WriteString("- 若已有内容足够回答，直接整合并说明不确定性；不要无意义重复同一个失败调用。")
	return sb.String()
}

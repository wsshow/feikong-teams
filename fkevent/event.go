// Package fkevent 提供智能体事件的处理和分发机制。
// 支持流式和非流式消息、工具调用、动作事件的统一处理。
package fkevent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// Event 统一的事件结构，承载各类智能体输出
type Event struct {
	Type             string            `json:"type"`
	AgentName        string            `json:"agent_name,omitempty"`
	RunPath          string            `json:"run_path,omitempty"`
	Content          string            `json:"content,omitempty"`
	Detail           string            `json:"detail,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"` // 推理模型思考内容
	ToolCalls        []schema.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"` // 工具结果对应的调用 ID
	ActionType       string            `json:"action_type,omitempty"`
	Error            string            `json:"error,omitempty"`
}

// formatRunPath 将运行路径格式化为字符串
func formatRunPath(runPath []adk.RunStep) string {
	return fmt.Sprintf("%v", runPath)
}

// callbackKey 用于在 context 中存储事件回调的 key
type callbackKey struct{}

// nonInteractiveKey 标记非交互模式（如 Web 服务），禁止终端 TUI
type nonInteractiveKey struct{}

// WithCallback 将事件回调绑定到 context
func WithCallback(ctx context.Context, cb func(Event) error) context.Context {
	return context.WithValue(ctx, callbackKey{}, cb)
}

// WithNonInteractive 标记当前 context 为非交互模式
func WithNonInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonInteractiveKey{}, true)
}

// IsNonInteractive 检查 context 是否为非交互模式
func IsNonInteractive(ctx context.Context) bool {
	v, _ := ctx.Value(nonInteractiveKey{}).(bool)
	return v
}

// getCallback 从 context 中获取事件回调函数
func getCallback(ctx context.Context) func(Event) error {
	if cb, ok := ctx.Value(callbackKey{}).(func(Event) error); ok {
		return cb
	}
	return nil
}

// ProcessAgentEvent 处理智能体事件，按顺序分发动作和消息输出
func ProcessAgentEvent(ctx context.Context, event *adk.AgentEvent) error {
	if event.Err != nil {
		if isContextCanceled(ctx, event.Err) {
			return nil
		}
		return handleEvent(ctx, Event{
			Type:      "error",
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			Error:     event.Err.Error(),
		})
	}

	// 先处理动作（如 transfer），再处理输出（如工具结果），保证显示顺序正确
	if event.Action != nil {
		if err := handleAction(ctx, event); err != nil {
			return err
		}
	}

	if event.Output != nil && event.Output.MessageOutput != nil {
		if err := handleMessageOutput(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// handleMessageOutput 处理消息输出，区分完整消息和流式消息
func handleMessageOutput(ctx context.Context, event *adk.AgentEvent) error {
	msgOutput := event.Output.MessageOutput

	if msg := msgOutput.Message; msg != nil {
		return handleRegularMessage(ctx, event, msg)
	}

	if stream := msgOutput.MessageStream; stream != nil {
		return handleStreamingMessage(ctx, event, stream)
	}

	return nil
}

// handleRegularMessage 处理非流式的完整消息
func handleRegularMessage(ctx context.Context, event *adk.AgentEvent, msg *schema.Message) error {
	eventType := "message"
	if msg.Role == schema.Tool {
		eventType = "tool_result"
	}

	nEvent := Event{
		Type:             eventType,
		AgentName:        event.AgentName,
		RunPath:          formatRunPath(event.RunPath),
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCallID:       msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		nEvent.ToolCalls = msg.ToolCalls
	}

	return handleEvent(ctx, nEvent)
}

// handleStreamingMessage 处理流式消息，通过 goroutine 异步接收以支持 context 取消
func handleStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream *schema.StreamReader[*schema.Message]) error {
	toolCallsMap := make(map[int][]*schema.Message) // 按 index 聚合工具调用分片
	toolCallStarted := make(map[int]bool)           // 记录已发送准备事件的工具调用

	// 工具参数增量发送的节流状态
	toolArgsBuffer := make(map[int]string) // 按 index 缓冲未发送的参数增量
	var lastArgsDeltaTime time.Time
	const argsDeltaInterval = 100 * time.Millisecond

	// 在独立 goroutine 中接收流数据，避免阻塞 context 取消检测
	type recvResult struct {
		chunk *schema.Message
		err   error
	}
	recvCh := make(chan recvResult, 1)

	go func() {
		defer close(recvCh)
		for {
			chunk, err := stream.Recv()
			recvCh <- recvResult{chunk, err}
			if err != nil {
				return
			}
		}
	}()

	cancelled := false
	for !cancelled {
		select {
		case <-ctx.Done():
			stream.Close()
			cancelled = true
		case r, ok := <-recvCh:
			if !ok || errors.Is(r.err, io.EOF) {
				cancelled = true
				break
			}
			if r.err != nil {
				if isContextCanceled(ctx, r.err) {
					cancelled = true
					break
				}
				return handleEvent(ctx, Event{
					Type:      "error",
					AgentName: event.AgentName,
					RunPath:   formatRunPath(event.RunPath),
					Error:     fmt.Sprintf("stream error: %v", r.err),
				})
			}

			chunk := r.chunk
			// 处理推理/思考内容（推理模型如 DeepSeek-R1、o1）
			if chunk.ReasoningContent != "" {
				if err := handleEvent(ctx, Event{
					Type:      "reasoning_chunk",
					AgentName: event.AgentName,
					RunPath:   formatRunPath(event.RunPath),
					Content:   chunk.ReasoningContent,
				}); err != nil {
					return err
				}
			}

			if chunk.Content != "" {
				eventType := "stream_chunk"
				if chunk.Role == schema.Tool {
					eventType = "tool_result_chunk"
				}
				if err := handleEvent(ctx, Event{
					Type:      eventType,
					AgentName: event.AgentName,
					RunPath:   formatRunPath(event.RunPath),
					Content:   chunk.Content,
				}); err != nil {
					return err
				}
			}

			if len(chunk.ToolCalls) > 0 {
				for _, tc := range chunk.ToolCalls {
					if tc.Index == nil {
						continue
					}
					if !toolCallStarted[*tc.Index] && tc.Function.Name != "" {
						toolCallStarted[*tc.Index] = true
						if err := handleEvent(ctx, Event{
							Type:      "tool_calls_preparing",
							AgentName: event.AgentName,
							RunPath:   formatRunPath(event.RunPath),
							ToolCalls: []schema.ToolCall{
								{Function: schema.FunctionCall{Name: tc.Function.Name}},
							},
						}); err != nil {
							return err
						}
					}
					toolCallsMap[*tc.Index] = append(toolCallsMap[*tc.Index], &schema.Message{
						Role: chunk.Role,
						ToolCalls: []schema.ToolCall{
							{
								ID:    tc.ID,
								Type:  tc.Type,
								Index: tc.Index,
								Function: schema.FunctionCall{
									Name:      tc.Function.Name,
									Arguments: tc.Function.Arguments,
								},
							},
						},
					})

					// 缓冲参数增量，节流发送
					if tc.Function.Arguments != "" {
						toolArgsBuffer[*tc.Index] += tc.Function.Arguments
					}
				}

				// 节流发送参数增量
				now := time.Now()
				if now.Sub(lastArgsDeltaTime) >= argsDeltaInterval && len(toolArgsBuffer) > 0 {
					for idx, delta := range toolArgsBuffer {
						if delta == "" {
							continue
						}
						if err := handleEvent(ctx, Event{
							Type:      "tool_calls_args_delta",
							AgentName: event.AgentName,
							RunPath:   formatRunPath(event.RunPath),
							Content:   delta,
							Detail:    fmt.Sprintf("%d", idx),
						}); err != nil {
							return err
						}
					}
					toolArgsBuffer = make(map[int]string)
					lastArgsDeltaTime = now
				}
			}
		}
	}

	// 发送最后一批缓冲的参数增量
	if len(toolArgsBuffer) > 0 {
		for idx, delta := range toolArgsBuffer {
			if delta == "" {
				continue
			}
			_ = handleEvent(ctx, Event{
				Type:      "tool_calls_args_delta",
				AgentName: event.AgentName,
				RunPath:   formatRunPath(event.RunPath),
				Content:   delta,
				Detail:    fmt.Sprintf("%d", idx),
			})
		}
	}

	// context 取消时跳过后续工具调用处理
	if ctx.Err() != nil {
		return nil
	}

	// 合并所有工具调用分片，按 index 排序后统一发送
	indices := make([]int, 0, len(toolCallsMap))
	for idx := range toolCallsMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var allToolCalls []schema.ToolCall
	for _, idx := range indices {
		concatenatedMsg, err := schema.ConcatMessages(toolCallsMap[idx])
		if err != nil {
			return err
		}
		allToolCalls = append(allToolCalls, concatenatedMsg.ToolCalls...)
	}

	if len(allToolCalls) > 0 {
		if err := handleEvent(ctx, Event{
			Type:      "tool_calls",
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			ToolCalls: allToolCalls,
		}); err != nil {
			return err
		}
	}

	return nil
}

// handleAction 处理智能体动作事件（转发、中断、退出）
func handleAction(ctx context.Context, event *adk.AgentEvent) error {
	action := event.Action

	if action.TransferToAgent != nil {
		return handleEvent(ctx, Event{
			Type:       "action",
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: "transfer",
			Content:    fmt.Sprintf("Transfer to agent: %s", action.TransferToAgent.DestAgentName),
		})
	}

	if action.Interrupted != nil {
		for _, ic := range action.Interrupted.InterruptContexts {
			content := fmt.Sprintf("%v", ic.Info)
			if stringer, ok := ic.Info.(fmt.Stringer); ok {
				content = stringer.String()
			}

			if err := handleEvent(ctx, Event{
				Type:       "action",
				AgentName:  event.AgentName,
				RunPath:    formatRunPath(event.RunPath),
				ActionType: "interrupted",
				Content:    content,
			}); err != nil {
				return err
			}
		}
	}

	if action.Exit {
		return handleEvent(ctx, Event{
			Type:       "action",
			AgentName:  event.AgentName,
			RunPath:    formatRunPath(event.RunPath),
			ActionType: "exit",
			Content:    "Agent execution completed",
		})
	}

	return nil
}

// isContextCanceled 判断是否为 context 取消导致的错误，这类错误不需要展示给用户
func isContextCanceled(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded")
}

// handleEvent 分发事件到 context 中的回调，无回调时仅打印
func handleEvent(ctx context.Context, event Event) error {
	if cb := getCallback(ctx); cb != nil {
		return cb(event)
	}
	PrintEvent(event)
	return nil
}

// DispatchEvent 向 context 中的回调分发事件
func DispatchEvent(ctx context.Context, event Event) error {
	return handleEvent(ctx, event)
}

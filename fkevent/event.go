package fkevent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Event struct {
	Type       string            `json:"type"`
	AgentName  string            `json:"agent_name,omitempty"`
	RunPath    string            `json:"run_path,omitempty"`
	Content    string            `json:"content,omitempty"`
	Detail     string            `json:"detail,omitempty"`
	ToolCalls  []schema.ToolCall `json:"tool_calls,omitempty"`
	ActionType string            `json:"action_type,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func formatRunPath(runPath []adk.RunStep) string {
	return fmt.Sprintf("%v", runPath)
}

type callbackKey struct{}

// WithCallback 将事件回调绑定到 context
func WithCallback(ctx context.Context, cb func(Event) error) context.Context {
	return context.WithValue(ctx, callbackKey{}, cb)
}

func getCallback(ctx context.Context) func(Event) error {
	if cb, ok := ctx.Value(callbackKey{}).(func(Event) error); ok {
		return cb
	}
	return nil
}

func ProcessAgentEvent(ctx context.Context, event *adk.AgentEvent) error {
	if event.Err != nil {
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

func handleRegularMessage(ctx context.Context, event *adk.AgentEvent, msg *schema.Message) error {
	eventType := "message"
	if msg.Role == schema.Tool {
		eventType = "tool_result"
	}

	nEvent := Event{
		Type:      eventType,
		AgentName: event.AgentName,
		RunPath:   formatRunPath(event.RunPath),
		Content:   msg.Content,
	}

	if len(msg.ToolCalls) > 0 {
		nEvent.ToolCalls = msg.ToolCalls
	}

	return handleEvent(ctx, nEvent)
}

func handleStreamingMessage(ctx context.Context, event *adk.AgentEvent, stream *schema.StreamReader[*schema.Message]) error {
	toolCallsMap := make(map[int][]*schema.Message)
	toolCallStarted := make(map[int]bool)

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
				return handleEvent(ctx, Event{
					Type:      "error",
					AgentName: event.AgentName,
					RunPath:   formatRunPath(event.RunPath),
					Error:     fmt.Sprintf("stream error: %v", r.err),
				})
			}

			chunk := r.chunk
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
				}
			}
		}
	}

	// context 取消时跳过后续处理
	if ctx.Err() != nil {
		return nil
	}

	// 按 index 顺序处理工具调用
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

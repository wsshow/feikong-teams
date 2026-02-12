package fkevent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Event struct {
	Type       string            `json:"type"`
	AgentName  string            `json:"agent_name,omitempty"`
	RunPath    string            `json:"run_path,omitempty"`
	Content    string            `json:"content,omitempty"`
	ToolCalls  []schema.ToolCall `json:"tool_calls,omitempty"`
	ActionType string            `json:"action_type,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func formatRunPath(runPath []adk.RunStep) string {
	return fmt.Sprintf("%v", runPath)
}

// callbackKey 用于在 context 中存储回调函数，支持并发安全
type callbackKey struct{}

// WithCallback 将回调函数绑定到 context，每个 goroutine 可拥有独立的回调
func WithCallback(ctx context.Context, cb func(Event) error) context.Context {
	return context.WithValue(ctx, callbackKey{}, cb)
}

// getCallback 从 context 中获取回调函数
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

	if event.Output != nil && event.Output.MessageOutput != nil {
		if err := handleMessageOutput(ctx, event); err != nil {
			return err
		}
	}

	if event.Action != nil {
		if err := handleAction(ctx, event); err != nil {
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
	toolCallStarted := make(map[int]bool) // 记录哪些工具调用已经发送了开始提示

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return handleEvent(ctx, Event{
				Type:      "error",
				AgentName: event.AgentName,
				RunPath:   formatRunPath(event.RunPath),
				Error:     fmt.Sprintf("stream error: %v", err),
			})
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
				if tc.Index != nil {
					// 如果这是该工具调用的第一个 chunk，立即显示准备提示
					if !toolCallStarted[*tc.Index] && tc.Function.Name != "" {
						toolCallStarted[*tc.Index] = true
						if err := handleEvent(ctx, Event{
							Type:      "tool_calls_preparing",
							AgentName: event.AgentName,
							RunPath:   formatRunPath(event.RunPath),
							ToolCalls: []schema.ToolCall{
								{
									Function: schema.FunctionCall{
										Name: tc.Function.Name,
									},
								},
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

	for _, msgs := range toolCallsMap {
		concatenatedMsg, err := schema.ConcatMessages(msgs)
		if err != nil {
			return err
		}

		if err := handleEvent(ctx, Event{
			Type:      "tool_calls",
			AgentName: event.AgentName,
			RunPath:   formatRunPath(event.RunPath),
			ToolCalls: concatenatedMsg.ToolCalls,
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

// handleEvent 从 context 中获取回调函数，无回调时走默认的记录+打印逻辑
func handleEvent(ctx context.Context, event Event) error {
	if cb := getCallback(ctx); cb != nil {
		return cb(event)
	}
	RecordEventWithHistory(event)
	return nil
}

package hooks

import (
	"context"
	"fmt"

	"fkteams/agentcore"
)

// InvokeBeforeRun 执行运行前 hook，并返回可能被改写的输入。
func (b *Bus) InvokeBeforeRun(ctx context.Context, input agentcore.TurnInput) (agentcore.TurnInput, error) {
	result, err := b.Invoke(ctx, Invocation{
		HookPoint: HookBeforeRun,
		Payload:   BeforeRunPayload{Input: input},
	})
	if err != nil {
		return input, err
	}
	if payload, ok := result.Payload.(BeforeRunPayload); ok {
		input = payload.Input
	}
	return input, nil
}

// InvokeAfterRun 执行运行结束 hook。
func (b *Bus) InvokeAfterRun(ctx context.Context, input agentcore.TurnInput, result *agentcore.RunResult, runErr error) error {
	_, err := b.Invoke(ctx, Invocation{
		HookPoint: HookAfterRun,
		Payload: AfterRunPayload{
			Input:  input,
			Result: result,
			Error:  runErr,
		},
	})
	return err
}

// InvokeEvent 执行事件 hook，返回改写后的事件以及是否继续分发。
func (b *Bus) InvokeEvent(ctx context.Context, event agentcore.Event) (agentcore.Event, bool, error) {
	result, err := b.Invoke(ctx, Invocation{
		HookPoint: HookOnEvent,
		Payload:   EventPayload{Event: event},
	})
	if err != nil {
		return event, false, err
	}
	if payload, ok := result.Payload.(EventPayload); ok {
		event = payload.Event
	}
	if result.Action == ActionSkip || result.Action == ActionReject {
		return event, false, nil
	}
	return event, true, nil
}

// InvokeBeforeToolCall 执行工具调用前 hook，并返回改写后的工具调用载荷。
func (b *Bus) InvokeBeforeToolCall(ctx context.Context, payload BeforeToolCallPayload) (BeforeToolCallPayload, error) {
	result, err := b.Invoke(ctx, Invocation{
		HookPoint: HookBeforeToolCall,
		Payload:   payload,
	})
	if err != nil {
		return payload, err
	}
	if err := actionError(result, "tool call"); err != nil {
		return payload, err
	}
	if next, ok := result.Payload.(BeforeToolCallPayload); ok {
		payload = next
	}
	return payload, nil
}

// InvokeAfterToolCall 执行工具调用后 hook。
func (b *Bus) InvokeAfterToolCall(ctx context.Context, payload AfterToolCallPayload) error {
	_, err := b.Invoke(ctx, Invocation{
		HookPoint: HookAfterToolCall,
		Payload:   payload,
	})
	return err
}

// InvokeBeforeModelRequest 执行模型请求前 hook，并返回可能被改写的消息。
func (b *Bus) InvokeBeforeModelRequest(ctx context.Context, messages []agentcore.Message) ([]agentcore.Message, error) {
	result, err := b.Invoke(ctx, Invocation{
		HookPoint: HookBeforeModelRequest,
		Payload:   BeforeModelRequestPayload{Messages: messages},
	})
	if err != nil {
		return messages, err
	}
	if err := actionError(result, "model request"); err != nil {
		return messages, err
	}
	if payload, ok := result.Payload.(BeforeModelRequestPayload); ok {
		messages = payload.Messages
	}
	return messages, nil
}

// InvokeAfterModelResponse 执行模型响应后 hook。
func (b *Bus) InvokeAfterModelResponse(ctx context.Context, payload AfterModelResponsePayload) error {
	_, err := b.Invoke(ctx, Invocation{
		HookPoint: HookAfterModelResponse,
		Payload:   payload,
	})
	return err
}

func actionError(result Result, subject string) error {
	switch result.Action {
	case ActionReject:
		if result.Message != "" {
			return fmt.Errorf("%s rejected by hook: %s", subject, result.Message)
		}
		return fmt.Errorf("%s rejected by hook", subject)
	case ActionSkip:
		return fmt.Errorf("%s skipped by hook", subject)
	default:
		return nil
	}
}

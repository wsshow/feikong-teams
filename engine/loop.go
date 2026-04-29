package engine

import (
	"context"
	"fkteams/fkevent"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

// runLoop 事件循环，处理迭代和中断/恢复
func (e *Engine) runLoop(ctx context.Context, messages []adk.Message, handler InterruptHandler) (*adk.AgentEvent, error) {
	iter := e.runner.Run(ctx, messages, adk.WithCheckPointID(e.checkpointID))
	for {
		lastEvent, err := drainEvents(ctx, iter)
		if err != nil {
			return lastEvent, err
		}

		if lastEvent != nil && lastEvent.Action != nil && lastEvent.Action.Interrupted != nil {
			interrupts := lastEvent.Action.Interrupted.InterruptContexts
			if len(interrupts) > 0 && handler != nil {
				targets, handlerErr := handler(ctx, interrupts)
				if handlerErr != nil {
					return lastEvent, handlerErr
				}
				resumeIter, resumeErr := e.runner.ResumeWithParams(ctx, e.checkpointID, &adk.ResumeParams{
					Targets: targets,
				})
				if resumeErr != nil {
					return lastEvent, fmt.Errorf("resume failed: %w", resumeErr)
				}
				iter = resumeIter
				continue
			}
		}
		return lastEvent, nil
	}
}

// drainEvents 遍历迭代器中所有事件，逐个调用 ProcessAgentEvent
func drainEvents(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent]) (*adk.AgentEvent, error) {
	var lastEvent *adk.AgentEvent
	for {
		select {
		case <-ctx.Done():
			return lastEvent, ctx.Err()
		default:
		}

		event, ok := iter.Next()
		if !ok {
			return lastEvent, nil
		}
		lastEvent = event
		if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
			return lastEvent, err
		}
	}
}

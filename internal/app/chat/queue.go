package chat

import (
	"context"
	"errors"
	"fmt"

	"fkteams/internal/app/chat/taskstream"
)

// QueueRequest 描述同一会话的连续回合。Next 只转换已消费输入，Finish 负责会话级收尾。
type QueueRequest struct {
	Stream *taskstream.Stream
	First  TurnRequest
	Next   func(taskstream.QueuedMessage) (TurnRequest, error)
	Finish func(error) error
}

// RunQueued 串行执行回合，原子地封闭空队列，完成持久化后再结束任务。
func (s *Service) RunQueued(ctx context.Context, req QueueRequest) (err error) {
	if req.Stream == nil || req.Next == nil {
		return fmt.Errorf("chat queue requires a stream and input builder")
	}
	defer func() {
		if req.Finish != nil {
			err = errors.Join(err, req.Finish(err))
		}
		req.Stream.SetStatus(string(CompletionStatus(err)))
	}()
	current := req.First
	for {
		if _, err = s.RunTurn(ctx, current); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		queued, ok := req.Stream.DequeueNextMessageOrSeal()
		if !ok {
			if req.Stream.Status() == "cancelled" {
				return context.Canceled
			}
			return nil
		}
		current, err = req.Next(queued)
		if err != nil {
			return err
		}
	}
}

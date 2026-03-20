package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"time"

	"fkteams/agents/retry/generic"
	"fkteams/log"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrExceedMaxRetries = errors.New("exceeds max retries")
)

type RetryExhaustedError struct {
	LastErr      error
	TotalRetries int
}

func (e *RetryExhaustedError) Error() string {
	if e.LastErr != nil {
		return fmt.Sprintf("exceeds max retries: last error: %v", e.LastErr)
	}
	return "exceeds max retries"
}

func (e *RetryExhaustedError) Unwrap() error {
	return ErrExceedMaxRetries
}

type WillRetryError struct {
	ErrStr       string
	RetryAttempt int
	err          error
}

func (e *WillRetryError) Error() string {
	return e.ErrStr
}

func (e *WillRetryError) Unwrap() error {
	return e.err
}

func init() {
	schema.RegisterName[*WillRetryError]("chatmodel_will_retry_error")
}

type ModelRetryConfig struct {
	MaxRetries  int
	IsRetryAble func(ctx context.Context, err error) bool
	BackoffFunc func(ctx context.Context, attempt int) time.Duration
}

func defaultIsRetryAble(_ context.Context, err error) bool {
	return err != nil
}

func defaultBackoff(_ context.Context, attempt int) time.Duration {
	baseDelay := 100 * time.Millisecond
	maxDelay := 10 * time.Second

	if attempt <= 0 {
		return baseDelay
	}

	if attempt > 7 {
		return maxDelay + time.Duration(rand.Int63n(int64(maxDelay/2)))
	}

	delay := baseDelay * time.Duration(1<<uint(attempt-1))
	if delay > maxDelay {
		delay = maxDelay
	}

	jitter := time.Duration(rand.Int63n(int64(delay / 2)))
	return delay + jitter
}

func genErrWrapper(ctx context.Context, config ModelRetryConfig, info streamRetryInfo) func(error) error {
	return func(err error) error {
		isRetryAble := config.IsRetryAble == nil || config.IsRetryAble(ctx, err)
		hasRetriesLeft := info.attempt < config.MaxRetries

		if isRetryAble && hasRetriesLeft {
			return &WillRetryError{ErrStr: err.Error(), RetryAttempt: info.attempt, err: err}
		}
		return err
	}
}

type retryChatModel struct {
	inner                 model.ToolCallingChatModel
	config                *ModelRetryConfig
	innerHandlesCallbacks bool
}

func NewRetryChatModel(inner model.ToolCallingChatModel, config *ModelRetryConfig) *retryChatModel {
	innerHandlesCallbacks := false
	if ch, ok := inner.(components.Checker); ok {
		innerHandlesCallbacks = ch.IsCallbacksEnabled()
	}
	return &retryChatModel{inner: inner, config: config, innerHandlesCallbacks: innerHandlesCallbacks}
}

func (r *retryChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	newInner, err := r.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	innerHandlesCallbacks := false
	if ch, ok := newInner.(components.Checker); ok {
		innerHandlesCallbacks = ch.IsCallbacksEnabled()
	}
	return &retryChatModel{inner: newInner, config: r.config, innerHandlesCallbacks: innerHandlesCallbacks}, nil
}

func (r *retryChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	isRetryAble := r.config.IsRetryAble
	if isRetryAble == nil {
		isRetryAble = defaultIsRetryAble
	}
	backoffFunc := r.config.BackoffFunc
	if backoffFunc == nil {
		backoffFunc = defaultBackoff
	}

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		var out *schema.Message
		var err error

		if r.innerHandlesCallbacks {
			out, err = r.inner.Generate(ctx, input, opts...)
		} else {
			out, err = r.generateWithProxyCallbacks(ctx, input, opts...)
		}

		if err == nil {
			return out, nil
		}

		if !isRetryAble(ctx, err) {
			return nil, err
		}

		lastErr = err
		if attempt < r.config.MaxRetries {
			log.Warnf("retrying ChatModel.Generate (attempt %d/%d): %v", attempt+1, r.config.MaxRetries, err)
			time.Sleep(backoffFunc(ctx, attempt+1))
		}
	}

	return nil, &RetryExhaustedError{LastErr: lastErr, TotalRetries: r.config.MaxRetries}
}

func (r *retryChatModel) generateWithProxyCallbacks(ctx context.Context,
	input []*schema.Message, opts ...model.Option) (*schema.Message, error) {

	ctx = callbacks.EnsureRunInfo(ctx, r.GetType(), components.ComponentOfChatModel)
	nCtx := callbacks.OnStart(ctx, &model.CallbackInput{Messages: input})

	out, err := r.inner.Generate(nCtx, input, opts...)
	if err != nil {
		callbacks.OnError(nCtx, err)
		return nil, err
	}

	callbacks.OnEnd(nCtx, &model.CallbackOutput{Message: out})
	return out, nil
}

type streamRetryKey struct{}

type streamRetryInfo struct {
	attempt int // first request is 0, first retry is 1
}

func getStreamRetryInfo(ctx context.Context) (*streamRetryInfo, bool) {
	info, ok := ctx.Value(streamRetryKey{}).(*streamRetryInfo)
	return info, ok
}

func (r *retryChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
	*schema.StreamReader[*schema.Message], error) {

	isRetryAble := r.config.IsRetryAble
	if isRetryAble == nil {
		isRetryAble = defaultIsRetryAble
	}
	backoffFunc := r.config.BackoffFunc
	if backoffFunc == nil {
		backoffFunc = defaultBackoff
	}

	retryInfo := &streamRetryInfo{}
	ctx = context.WithValue(ctx, streamRetryKey{}, retryInfo)

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		retryInfo.attempt = attempt
		var stream *schema.StreamReader[*schema.Message]
		var err error

		if r.innerHandlesCallbacks {
			stream, err = r.inner.Stream(ctx, input, opts...)
		} else {
			stream, err = r.streamWithProxyCallbacks(ctx, input, opts...)
		}

		if err != nil {
			if !isRetryAble(ctx, err) {
				return nil, err
			}
			lastErr = err
			if attempt < r.config.MaxRetries {
				log.Warnf("retrying ChatModel.Stream (attempt %d/%d): %v", attempt+1, r.config.MaxRetries, err)
				time.Sleep(backoffFunc(ctx, attempt+1))
			}
			continue
		}

		copies := stream.Copy(2)
		checkCopy := copies[0]
		returnCopy := copies[1]

		streamErr := consumeStreamForError(checkCopy)
		if streamErr == nil {
			return returnCopy, nil
		}

		returnCopy.Close()
		if !isRetryAble(ctx, streamErr) {
			return nil, streamErr
		}

		lastErr = streamErr
		if attempt < r.config.MaxRetries {
			log.Warnf("retrying ChatModel.Stream (attempt %d/%d): %v", attempt+1, r.config.MaxRetries, streamErr)
			time.Sleep(backoffFunc(ctx, attempt+1))
		}
	}

	return nil, &RetryExhaustedError{LastErr: lastErr, TotalRetries: r.config.MaxRetries}
}

func (r *retryChatModel) streamWithProxyCallbacks(ctx context.Context,
	input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {

	ctx = callbacks.EnsureRunInfo(ctx, r.GetType(), components.ComponentOfChatModel)
	nCtx := callbacks.OnStart(ctx, &model.CallbackInput{Messages: input})

	stream, err := r.inner.Stream(nCtx, input, opts...)
	if err != nil {
		callbacks.OnError(nCtx, err)
		return nil, err
	}

	out := schema.StreamReaderWithConvert(stream, func(m *schema.Message) (*model.CallbackOutput, error) {
		return &model.CallbackOutput{Message: m}, nil
	})
	_, out = callbacks.OnEndWithStreamOutput(nCtx, out)
	return schema.StreamReaderWithConvert(out, func(o *model.CallbackOutput) (*schema.Message, error) {
		return o.Message, nil
	}), nil
}

func consumeStreamForError(stream *schema.StreamReader[*schema.Message]) error {
	defer stream.Close()
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (r *retryChatModel) GetType() string {
	if gt, ok := r.inner.(components.Typer); ok {
		return gt.GetType()
	}
	return generic.ParseTypeName(reflect.ValueOf(r.inner))
}

func (r *retryChatModel) IsCallbacksEnabled() bool { return true }

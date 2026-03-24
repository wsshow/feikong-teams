// Package retry 提供 ChatModel 的重试包装器，支持 Generate 和 Stream 的自动重试。
package retry

import (
	"context"
	"errors"
	"fmt"
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

// ErrExceedMaxRetries 所有重试次数耗尽时返回的哨兵错误，可通过 errors.Is 匹配
var ErrExceedMaxRetries = errors.New("exceeds max retries")

// RetryExhaustedError 重试耗尽时的详细错误，包含最后一次的原始错误
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

// ModelRetryConfig 重试配置
type ModelRetryConfig struct {
	MaxRetries  int
	IsRetryAble func(ctx context.Context, err error) bool
	BackoffFunc func(ctx context.Context, attempt int) time.Duration
}

func defaultIsRetryAble(_ context.Context, err error) bool {
	return err != nil
}

// defaultBackoff 指数退避 + 随机抖动，基础 100ms，上限 10s
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

// retryChatModel 为 ToolCallingChatModel 添加自动重试能力。
// Generate 对完整请求重试；Stream 仅对初始连接失败重试，流建立后直接返回以保持流式输出。
type retryChatModel struct {
	inner                 model.ToolCallingChatModel
	config                *ModelRetryConfig
	innerHandlesCallbacks bool
}

// NewRetryChatModel 创建带重试能力的 ChatModel 包装器
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

// Generate 带重试的非流式生成，失败时按退避策略重试
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

// Stream 带重试的流式生成。仅在建立连接阶段重试，流成功建立后直接返回以保持真正的流式输出。
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

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
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

		return stream, nil
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

func (r *retryChatModel) GetType() string {
	if gt, ok := r.inner.(components.Typer); ok {
		return gt.GetType()
	}
	return generic.ParseTypeName(reflect.ValueOf(r.inner))
}

func (r *retryChatModel) IsCallbacksEnabled() bool { return true }

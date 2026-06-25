// Package events 提供运行时无关的事件分发层。
package events

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/hooks"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type callbackKey struct{}
type nonInteractiveKey struct{}

var globalEventSequence int64

// WithCallback 将事件回调绑定到 context。
func WithCallback(ctx context.Context, cb func(Event) error) context.Context {
	return context.WithValue(ctx, callbackKey{}, cb)
}

func getCallback(ctx context.Context) func(Event) error {
	if cb, ok := ctx.Value(callbackKey{}).(func(Event) error); ok {
		return cb
	}
	return nil
}

// WithNonInteractive 标记 context 为非交互模式。
func WithNonInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonInteractiveKey{}, true)
}

// IsNonInteractive 判断 context 是否为非交互模式。
func IsNonInteractive(ctx context.Context) bool {
	v, _ := ctx.Value(nonInteractiveKey{}).(bool)
	return v
}

// NormalizeEvent 补齐事件公共元数据。
func NormalizeEvent(event Event) Event {
	if event.Sequence == 0 {
		event.Sequence = atomic.AddInt64(&globalEventSequence, 1)
	}
	if event.EventID == "" {
		event.EventID = fmt.Sprintf("evt_%d", event.Sequence)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return event
}

func IsMemberEvent(event Event) bool {
	return event.MemberCallID != ""
}

// DispatchEvent 标准化事件并发送到 context 回调。
func DispatchEvent(ctx context.Context, event Event) error {
	event = NormalizeEvent(event)
	event, emit, err := hooks.FromContext(ctx).InvokeEvent(ctx, event)
	if err != nil {
		return err
	}
	if !emit {
		return nil
	}
	if cb := getCallback(ctx); cb != nil {
		return cb(event)
	}
	return nil
}

// Dispatch 将 context 适配为 EventSink。
func Dispatch(ctx context.Context) runtimeport.EventSink {
	return func(event Event) error {
		return DispatchEvent(ctx, event)
	}
}

func IsInternalToolName(name string) bool {
	return name == "continue_output"
}

func IsInternalContinueContent(content string) bool {
	return strings.Contains(content, "Your previous text output was truncated") ||
		strings.Contains(content, "Your previous tool call was truncated")
}

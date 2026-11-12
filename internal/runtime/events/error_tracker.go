package events

import (
	"errors"
	"sync"
)

// ErrorTracker 记录主运行已发布的错误，避免返回值在收尾时重复展示和持久化。
// 零值即可使用；成员错误不会抵消主运行错误，合并错误中的新故障仍会保留。
type ErrorTracker struct {
	mu       sync.Mutex
	reported map[string]bool
}

// Record 记住已经发布的主运行错误事件。
func (t *ErrorTracker) Record(event Event) {
	if event.Type != EventError || event.MemberCallID != "" || event.ParentToolCallID != "" {
		return
	}
	raw := event.Error
	if raw == "" {
		raw = event.Content
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.reported == nil {
		t.reported = make(map[string]bool)
	}
	t.reported[raw] = true
}

// Unreported 返回尚未发布的错误；只拆分合并错误，保留普通包装错误的上下文。
func (t *ErrorTracker) Unreported(err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.unreported(err)
}

func (t *ErrorTracker) unreported(err error) error {
	if err == nil || t.reported[err.Error()] {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var remaining []error
		for _, child := range joined.Unwrap() {
			if next := t.unreported(child); next != nil {
				remaining = append(remaining, next)
			}
		}
		return errors.Join(remaining...)
	}
	return err
}

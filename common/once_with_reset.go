package common

import "sync"

// OnceWithReset 可重置的单次执行器
type OnceWithReset struct {
	triggered bool
	mu        sync.Mutex
}

// NewOnceWithReset 创建可重置的单次执行器
func NewOnceWithReset() *OnceWithReset {
	return &OnceWithReset{}
}

// Do 执行函数，仅首次调用时执行
func (o *OnceWithReset) Do(fn func()) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.triggered {
		o.triggered = true
		fn()
	}
}

// Reset 重置执行状态，使 Do 可以再次执行
func (o *OnceWithReset) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.triggered = false
}

// IsTriggered 返回是否已执行过
func (o *OnceWithReset) IsTriggered() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.triggered
}

package common

import "sync"

type OnceWithReset struct {
	triggered bool
	mu        sync.Mutex
}

func NewOnceWithReset() *OnceWithReset {
	return &OnceWithReset{}
}

func (o *OnceWithReset) Do(fn func()) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.triggered {
		o.triggered = true
		fn()
	}
}

func (o *OnceWithReset) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.triggered = false
}

func (o *OnceWithReset) IsTriggered() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.triggered
}

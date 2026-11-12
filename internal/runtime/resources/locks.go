package resources

import (
	"context"
	"sync"
)

// KeyedLocks 协调同一应用内共享资源；读取可并行，写入独占且优先于后到的读取。
// 锁等待支持取消，空闲键会自动回收。
type KeyedLocks struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}
type lockEntry struct {
	readers        int
	writer         bool
	waitingWriters int
	refs           int
	changed        chan struct{}
}

// NewKeyedLocks 创建实例级资源锁集合。
func NewKeyedLocks() *KeyedLocks { return &KeyedLocks{entries: make(map[string]*lockEntry)} }

// Acquire 等待独占资源，覆盖完整的读改写事务。
func (l *KeyedLocks) Acquire(ctx context.Context, key string) (func(), error) {
	return l.acquire(ctx, key, false)
}

// AcquireRead 等待资源的共享读取租约。
func (l *KeyedLocks) AcquireRead(ctx context.Context, key string) (func(), error) {
	return l.acquire(ctx, key, true)
}

func (l *KeyedLocks) acquire(ctx context.Context, key string, read bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*lockEntry)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &lockEntry{changed: make(chan struct{})}
		l.entries[key] = entry
	}
	entry.refs++
	if !read {
		entry.waitingWriters++
	}
	notify := func() { close(entry.changed); entry.changed = make(chan struct{}) }
	drop := func() {
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			if !read {
				entry.waitingWriters--
			}
			drop()
			notify()
			l.mu.Unlock()
			return nil, err
		}
		available := !entry.writer && ((read && entry.waitingWriters == 0) || (!read && entry.readers == 0))
		if available {
			if read {
				entry.readers++
			} else {
				entry.waitingWriters--
				entry.writer = true
			}
			break
		}
		changed := entry.changed
		l.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		l.mu.Lock()
	}
	l.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if read {
				entry.readers--
			} else {
				entry.writer = false
			}
			drop()
			notify()
			l.mu.Unlock()
		})
	}, nil
}

// Serialize 在独占资源租约内调用有类型的处理函数。
func Serialize[I, O any](locks *KeyedLocks, key string, handler func(context.Context, *I) (O, error)) func(context.Context, *I) (O, error) {
	return guard(locks, key, false, handler)
}

// Read 在共享读取租约内调用有类型的处理函数。
func Read[I, O any](locks *KeyedLocks, key string, handler func(context.Context, *I) (O, error)) func(context.Context, *I) (O, error) {
	return guard(locks, key, true, handler)
}

func guard[I, O any](locks *KeyedLocks, key string, read bool, handler func(context.Context, *I) (O, error)) func(context.Context, *I) (O, error) {
	return func(ctx context.Context, req *I) (O, error) {
		release, err := locks.acquire(ctx, key, read)
		if err != nil {
			var zero O
			return zero, err
		}
		defer release()
		return handler(ctx, req)
	}
}

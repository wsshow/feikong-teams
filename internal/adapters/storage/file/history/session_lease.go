package eventlog

import (
	"path/filepath"
	"sync"
)

type sessionLeaseState struct {
	active   int
	deleting bool
}

type sessionLeaseRegistry struct {
	mu     sync.Mutex
	cond   *sync.Cond
	states map[string]*sessionLeaseState
}

var sessionLeases = newSessionLeaseRegistry()

func newSessionLeaseRegistry() *sessionLeaseRegistry {
	registry := &sessionLeaseRegistry{states: make(map[string]*sessionLeaseState)}
	registry.cond = sync.NewCond(&registry.mu)
	return registry
}

// AcquireSessionLease 标记会话仍被队列或运行任务使用。
func AcquireSessionLease(historyDir, sessionID string) func() {
	key := sessionLeaseKey(historyDir, sessionID)
	sessionLeases.mu.Lock()
	state := sessionLeases.states[key]
	if state == nil {
		state = &sessionLeaseState{}
		sessionLeases.states[key] = state
	}
	for state.deleting {
		sessionLeases.cond.Wait()
	}
	state.active++
	sessionLeases.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			sessionLeases.mu.Lock()
			state := sessionLeases.states[key]
			if state != nil {
				state.active--
				if state.active == 0 && !state.deleting {
					delete(sessionLeases.states, key)
				}
			}
			sessionLeases.mu.Unlock()
		})
	}
}

// beginSessionDelete 在确认会话无使用者后阻止删除期间出现新租约。
func beginSessionDelete(historyDir, sessionID string) (func(), bool) {
	key := sessionLeaseKey(historyDir, sessionID)
	sessionLeases.mu.Lock()
	state := sessionLeases.states[key]
	if state != nil && (state.active > 0 || state.deleting) {
		sessionLeases.mu.Unlock()
		return nil, false
	}
	if state == nil {
		state = &sessionLeaseState{}
		sessionLeases.states[key] = state
	}
	state.deleting = true
	sessionLeases.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			sessionLeases.mu.Lock()
			state.deleting = false
			if state.active == 0 {
				delete(sessionLeases.states, key)
			}
			sessionLeases.cond.Broadcast()
			sessionLeases.mu.Unlock()
		})
	}, true
}

func sessionLeaseKey(historyDir, sessionID string) string {
	path := filepath.Join(historyDir, sessionID)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

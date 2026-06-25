package appstate

import (
	"context"
	"fkteams/internal/runtime/resources"
	"fkteams/log"
	"fkteams/memory"
	"sync"
)

type contextKey struct{}

// MemoryManager 描述运行时需要的长期记忆能力。
type MemoryManager interface {
	MemorySearcher
	MemoryCatalog
	MemoryExtractor
	MemoryLifecycle
}

// MemorySearcher 提供模型上下文注入需要的记忆检索能力。
type MemorySearcher interface {
	Search(query string, topK int) []memory.MemoryEntry
}

// MemoryCatalog 提供管理端需要的记忆维护能力。
type MemoryCatalog interface {
	List() []memory.MemoryEntry
	Delete(summary string) int
	Count() int
	Clear()
}

// MemoryExtractor 提供对话结束后的记忆提取能力。
type MemoryExtractor interface {
	ExtractAndStore(ctx context.Context, messages []memory.Message, sessionID string)
	FlushExtract(ctx context.Context, messages []memory.Message, sessionID string)
}

// MemoryLifecycle 提供记忆服务生命周期能力。
type MemoryLifecycle interface {
	ResetLLM(llm memory.LLMClient)
	Wait()
}

// State 保存单个应用实例的运行时依赖。
type State struct {
	mu      sync.RWMutex
	memory  MemoryManager
	cleaner *resources.Cleaner
}

// New 创建独立的应用运行时状态。
func New() *State {
	return &State{
		cleaner: resources.NewCleaner(),
	}
}

// WithState 把应用状态写入 context，供深层构造流程读取。
func WithState(ctx context.Context, state *State) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, state)
}

// FromContext 从 context 读取应用状态。
func FromContext(ctx context.Context) *State {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(contextKey{}).(*State)
	return state
}

// Memory 返回当前长期记忆管理器。
func (s *State) Memory() MemoryManager {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memory
}

// SetMemory 设置当前长期记忆管理器。
func (s *State) SetMemory(manager MemoryManager) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memory = manager
}

// Cleaner 返回进程生命周期资源清理器。
func (s *State) Cleaner() *resources.Cleaner {
	if s == nil {
		return resources.NewCleaner()
	}
	return s.cleaner
}

// RunProcessCleanup 执行并清空进程生命周期清理函数。
func (s *State) RunProcessCleanup() {
	if s == nil || s.cleaner == nil {
		return
	}
	if err := s.cleaner.ExecuteAndClear(); err != nil {
		log.Printf("[cleanup] 进程资源清理出错: %v", err)
	}
}

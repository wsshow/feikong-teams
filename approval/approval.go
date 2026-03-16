package approval

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

// Decision 审批决定常量
const (
	Reject      = 0 // 拒绝
	ApproveOnce = 1 // 允许一次
	ApproveItem = 2 // 该会话允许该项（命令/目录等）
	ApproveAll  = 3 // 该会话允许所有
)

// ErrRejected 用户明确拒绝操作
var ErrRejected = errors.New("用户拒绝了操作")

// MatchFunc 自定义匹配函数，判断 key 是否被 approved 集合覆盖。
// 默认精确匹配。文件工具可提供父目录遍历匹配。
type MatchFunc func(key string, approved map[string]bool) bool

// Store 会话级审批状态存储
type Store struct {
	mu      sync.RWMutex
	items   map[string]bool
	all     bool
	matcher MatchFunc
}

// NewStore 创建审批存储，matcher 可选（nil 则使用精确匹配）
func NewStore(matcher MatchFunc) *Store {
	return &Store{
		items:   make(map[string]bool),
		matcher: matcher,
	}
}

// IsApproved 检查 key 是否已被批准
func (s *Store) IsApproved(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.all {
		return true
	}
	if s.matcher != nil {
		return s.matcher(key, s.items)
	}
	return s.items[key]
}

// Approve 批准指定 key
func (s *Store) Approve(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = true
}

// SetApproveAll 批准所有
func (s *Store) SetApproveAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all = true
}

// Registry 审批注册表，管理多个命名的审批存储
type Registry struct {
	mu     sync.RWMutex
	stores map[string]*Store
}

// NewRegistry 创建审批注册表
func NewRegistry() *Registry {
	return &Registry{stores: make(map[string]*Store)}
}

// Register 注册审批存储
func (r *Registry) Register(name string, store *Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stores[name] = store
}

// Get 获取指定名称的审批存储
func (r *Registry) Get(name string) *Store {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stores[name]
}

type registryCtxKey struct{}

// WithRegistry 将审批注册表注入 context
func WithRegistry(ctx context.Context, reg *Registry) context.Context {
	return context.WithValue(ctx, registryCtxKey{}, reg)
}

// GetRegistry 从 context 获取审批注册表
func GetRegistry(ctx context.Context) *Registry {
	if v, ok := ctx.Value(registryCtxKey{}).(*Registry); ok {
		return v
	}
	return nil
}

// GetStore 从 context 获取指定名称的审批存储（便捷方法）
func GetStore(ctx context.Context, name string) *Store {
	if reg := GetRegistry(ctx); reg != nil {
		return reg.Get(name)
	}
	return nil
}

// StoreConfig 审批存储配置项
type StoreConfig struct {
	Name    string
	Matcher MatchFunc
}

// NewDefaultRegistry 创建并注册默认审批存储
func NewDefaultRegistry(configs ...StoreConfig) *Registry {
	reg := NewRegistry()
	for _, c := range configs {
		reg.Register(c.Name, NewStore(c.Matcher))
	}
	return reg
}

// Require 执行标准 HITL 审批流程。
//
// 返回值：
//   - nil: 已批准
//   - tool.Interrupt 错误: 需要 HITL 中断（调用方应直接返回此 error）
//   - ErrRejected: 用户拒绝（调用方可据此返回自定义响应）
func Require(ctx context.Context, storeName, key, info string) error {
	store := GetStore(ctx, storeName)

	// 1. 检查是否已批准
	if store != nil && store.IsApproved(key) {
		return nil
	}

	// 2. 检查是否从中断恢复
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if wasInterrupted {
		isTarget, hasData, decision := tool.GetResumeContext[int](ctx)
		if !isTarget {
			return tool.Interrupt(ctx, nil)
		}
		if hasData {
			switch decision {
			case ApproveOnce:
				return nil
			case ApproveItem:
				if store != nil {
					store.Approve(key)
				}
				return nil
			case ApproveAll:
				if store != nil {
					store.SetApproveAll()
				}
				return nil
			}
		}
		return fmt.Errorf("%w", ErrRejected)
	}

	// 3. 首次需要审批，触发 HITL 中断
	return tool.Interrupt(ctx, info)
}

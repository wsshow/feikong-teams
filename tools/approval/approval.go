package approval

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/cloudwego/eino/components/tool"
)

const (
	StoreCommand  = "command"
	StoreFile     = "file"
	StoreDispatch = "dispatch"
)

const (
	Reject      = 0
	ApproveOnce = 1
	ApproveItem = 2
	ApproveAll  = 3
)

var ErrRejected = errors.New("user rejected the operation")

type MatchFunc func(key string, approved map[string]bool) bool

func DirMatchFunc(key string, approved map[string]bool) bool {
	dir := key
	for {
		if approved[dir] {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}

type Store struct {
	items   map[string]bool
	all     bool
	matcher MatchFunc
}

func newStore(matcher MatchFunc) *Store {
	return &Store{items: make(map[string]bool), matcher: matcher}
}

func (s *Store) IsApproved(key string) bool {
	if s.all {
		return true
	}
	if s.matcher != nil {
		return s.matcher(key, s.items)
	}
	return s.items[key]
}

func (s *Store) approve(key string) { s.items[key] = true }
func (s *Store) setApproveAll()     { s.all = true }

type Registry struct {
	stores map[string]*Store
}

type StoreConfig struct {
	Name    string
	Matcher MatchFunc
}

func NewRegistry(configs ...StoreConfig) *Registry {
	r := &Registry{stores: make(map[string]*Store, len(configs))}
	for _, c := range configs {
		r.stores[c.Name] = newStore(c.Matcher)
	}
	return r
}

func (r *Registry) get(name string) *Store { return r.stores[name] }

// NewAutoApproveRegistry 创建一个所有操作均自动审批通过的 Registry。
// 用于子任务等无需人工确认的自主执行场景。
func NewAutoApproveRegistry() *Registry {
	r := NewRegistry(
		StoreConfig{Name: StoreCommand},
		StoreConfig{Name: StoreFile},
		StoreConfig{Name: StoreDispatch},
	)
	for _, s := range r.stores {
		s.setApproveAll()
	}
	return r
}

type registryCtxKey struct{}

func WithRegistry(ctx context.Context, reg *Registry) context.Context {
	return context.WithValue(ctx, registryCtxKey{}, reg)
}

func getStore(ctx context.Context, name string) *Store {
	if reg, ok := ctx.Value(registryCtxKey{}).(*Registry); ok {
		return reg.get(name)
	}
	return nil
}

func Require(ctx context.Context, storeName, key, info string) error {
	store := getStore(ctx, storeName)

	if store != nil && store.IsApproved(key) {
		return nil
	}

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
					store.approve(key)
				}
				return nil
			case ApproveAll:
				if store != nil {
					store.setApproveAll()
				}
				return nil
			}
		}
		return ErrRejected
	}

	return tool.Interrupt(ctx, info)
}

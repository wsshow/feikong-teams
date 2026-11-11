package share

import (
	"errors"
	"fkteams/internal/runtime/atomicfile"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	maxPersistedShareStoreBytes = 16 << 20
	maxPersistedShareEntries    = 10_000
)

var errShareStoreFull = errors.New("share store entry limit reached")

func readPersistedShareStore(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPersistedShareStoreBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPersistedShareStoreBytes {
		return nil, fmt.Errorf("share store file is too large")
	}
	return data, nil
}

func validatePersistedShareStoreSize(data []byte) error {
	if len(data) > maxPersistedShareStoreBytes {
		return fmt.Errorf("share store file is too large")
	}
	return nil
}

// ExpiresAtUnix 将未设置的有效期编码为零。
func ExpiresAtUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// entryStore 只复用并发、持久化和回滚机制；条目校验、复制与文件格式由具体存储提供。
type entryStore[T any] struct {
	mu       sync.RWMutex
	filePath string
	m        map[string]*T
	loadErr  error
	clone    func(*T) *T
	validate func(string, *T) error
	encode   func(map[string]*T) ([]byte, error)
}

func newEntryStore[T any](path string, clone func(*T) *T, validate func(string, *T) error, encode func(map[string]*T) ([]byte, error)) *entryStore[T] {
	return &entryStore[T]{filePath: path, m: make(map[string]*T), clone: clone, validate: validate, encode: encode}
}

// LoadError 返回构造时的加载错误，供组合层阻止不完整的服务启动。
func (s *entryStore[T]) LoadError() error { return s.loadErr }

// Path 返回存储路径。
func (s *entryStore[T]) Path() string { return s.filePath }

// Save 保存当前所有条目。
func (s *entryStore[T]) Save() error { return s.SaveTo(s.filePath) }

// SaveTo 导出当前条目，不改变后续写入的存储路径。
func (s *entryStore[T]) SaveTo(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveLockedTo(path)
}

func (s *entryStore[T]) saveLockedTo(path string) error {
	data, err := s.encode(s.m)
	if err != nil {
		return err
	}
	if err := validatePersistedShareStoreSize(data); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0644)
}

// Put 保存条目的独立副本；持久化失败时恢复内存状态。
func (s *entryStore[T]) Put(id string, entry *T) error {
	if err := s.validate(id, entry); err != nil {
		return err
	}
	copied := s.clone(entry)
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.m[id]
	if !existed && len(s.m) >= maxPersistedShareEntries {
		return errShareStoreFull
	}
	s.m[id] = copied
	if err := s.saveLockedTo(s.filePath); err != nil {
		if existed {
			s.m[id] = previous
		} else {
			delete(s.m, id)
		}
		return err
	}
	return nil
}

// Delete 删除一项授权，失败时保留原条目。
func (s *entryStore[T]) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.m[id]
	if !exists {
		return false, nil
	}
	return true, s.deleteLocked([]string{id})
}

// DeleteMany 原子删除一组授权，失败时恢复全部条目。
func (s *entryStore[T]) DeleteMany(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteLocked(ids)
}

func (s *entryStore[T]) deleteLocked(ids []string) error {
	previous := make(map[string]*T, len(ids))
	for _, id := range ids {
		if entry, ok := s.m[id]; ok {
			previous[id] = entry
			delete(s.m, id)
		}
	}
	if len(previous) == 0 {
		return nil
	}
	if err := s.saveLockedTo(s.filePath); err != nil {
		for id, entry := range previous {
			s.m[id] = entry
		}
		return err
	}
	return nil
}

// Lookup 返回独立副本，调用方不需要持有存储锁。
func (s *entryStore[T]) Lookup(id string) (*T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.m[id]
	if !ok {
		return nil, false
	}
	return s.clone(entry), true
}

// Snapshot 返回全部条目的独立副本。
func (s *entryStore[T]) Snapshot() map[string]*T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*T, len(s.m))
	for id, entry := range s.m {
		result[id] = s.clone(entry)
	}
	return result
}

package common

import (
	"fmt"
	"sync"
)

// CleanupFunc 定义清理函数类型
type CleanupFunc func() error

// ResourceCleaner 资源清理管理器
type ResourceCleaner struct {
	mu       sync.Mutex
	cleanups []CleanupFunc
	names    map[string]int // 用于按名称删除，存储索引
}

// NewResourceCleaner 创建新的资源清理器
func NewResourceCleaner() *ResourceCleaner {
	return &ResourceCleaner{
		cleanups: make([]CleanupFunc, 0),
		names:    make(map[string]int),
	}
}

// Add 添加一个清理函数（无名称）
func (rc *ResourceCleaner) Add(cleanup CleanupFunc) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cleanups = append(rc.cleanups, cleanup)
}

// AddNamed 添加一个带名称的清理函数
func (rc *ResourceCleaner) AddNamed(name string, cleanup CleanupFunc) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	idx := len(rc.cleanups)
	rc.cleanups = append(rc.cleanups, cleanup)
	rc.names[name] = idx
}

// Remove 删除指定名称的清理函数
func (rc *ResourceCleaner) Remove(name string) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	idx, exists := rc.names[name]
	if !exists {
		return false
	}

	// 将该位置设为 nil（不改变其他索引）
	if idx < len(rc.cleanups) {
		rc.cleanups[idx] = nil
	}
	delete(rc.names, name)
	return true
}

// RemoveLast 删除最后添加的清理函数
func (rc *ResourceCleaner) RemoveLast() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if len(rc.cleanups) == 0 {
		return false
	}

	rc.cleanups = rc.cleanups[:len(rc.cleanups)-1]
	return true
}

// Execute 执行所有清理函数（后进先出，类似 defer）
func (rc *ResourceCleaner) Execute() []error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	var errs []error

	// 反向执行（LIFO）
	for i := len(rc.cleanups) - 1; i >= 0; i-- {
		if rc.cleanups[i] != nil {
			if err := rc.safeExecute(rc.cleanups[i]); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// 清空所有清理函数
	rc.cleanups = make([]CleanupFunc, 0)
	rc.names = make(map[string]int)

	return errs
}

// safeExecute 安全执行清理函数，捕获 panic
func (rc *ResourceCleaner) safeExecute(cleanup CleanupFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during cleanup: %v", r)
		}
	}()

	return cleanup()
}

// ExecuteAndClear 执行所有清理函数并返回第一个错误
func (rc *ResourceCleaner) ExecuteAndClear() error {
	errs := rc.Execute()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// ExecuteNamed 执行指定名称的清理函数
func (rc *ResourceCleaner) ExecuteNamed(name string) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	idx, exists := rc.names[name]
	if !exists {
		return fmt.Errorf("cleanup function '%s' not found", name)
	}

	if idx >= len(rc.cleanups) || rc.cleanups[idx] == nil {
		return fmt.Errorf("cleanup function '%s' already executed or removed", name)
	}

	// 安全执行清理函数
	err := rc.safeExecute(rc.cleanups[idx])

	// 执行后移除
	rc.cleanups[idx] = nil
	delete(rc.names, name)

	return err
}

// ExecuteNamedKeep 执行指定名称的清理函数但保留（可重复执行）
func (rc *ResourceCleaner) ExecuteNamedKeep(name string) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	idx, exists := rc.names[name]
	if !exists {
		return fmt.Errorf("cleanup function '%s' not found", name)
	}

	if idx >= len(rc.cleanups) || rc.cleanups[idx] == nil {
		return fmt.Errorf("cleanup function '%s' already removed", name)
	}

	// 安全执行但不移除
	return rc.safeExecute(rc.cleanups[idx])
}

// Clear 清空所有清理函数但不执行
func (rc *ResourceCleaner) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cleanups = make([]CleanupFunc, 0)
	rc.names = make(map[string]int)
}

// Count 返回当前清理函数的数量
func (rc *ResourceCleaner) Count() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	count := 0
	for _, f := range rc.cleanups {
		if f != nil {
			count++
		}
	}
	return count
}

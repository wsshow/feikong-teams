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
}

// NewResourceCleaner 创建新的资源清理器
func NewResourceCleaner() *ResourceCleaner {
	return &ResourceCleaner{}
}

// Add 添加一个清理函数
func (rc *ResourceCleaner) Add(cleanup CleanupFunc) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cleanups = append(rc.cleanups, cleanup)
}

// ExecuteAndClear 执行所有清理函数（后进先出）并返回第一个错误
func (rc *ResourceCleaner) ExecuteAndClear() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	var firstErr error
	for i := len(rc.cleanups) - 1; i >= 0; i-- {
		if err := rc.safeExecute(rc.cleanups[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	rc.cleanups = nil
	return firstErr
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

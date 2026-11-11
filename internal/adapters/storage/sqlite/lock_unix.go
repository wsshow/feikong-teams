//go:build !windows

package sqlite

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// LockDirectory 防止多个新版进程同时访问同一业务数据目录。
func LockDirectory(path string) (func() error, error) {
	if err := validateLockPath(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("data directory is in use by another process: %w", err)
	}
	return file.Close, nil
}

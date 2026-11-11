package sqlite

import (
	"fmt"
	"golang.org/x/sys/windows"
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
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped); err != nil {
		file.Close()
		return nil, fmt.Errorf("data directory is in use by another process: %w", err)
	}
	return file.Close, nil
}

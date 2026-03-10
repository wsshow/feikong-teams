// Package utils 提供通用的文件系统工具函数
package utils

import "os"

// PathExists 检查路径是否存在
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// EnsureDir 确保目录存在，不存在则创建
func EnsureDir(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return err
}

// NotExistToMkdir 等同于 EnsureDir，确保目录存在
func NotExistToMkdir(path string) error {
	return EnsureDir(path)
}

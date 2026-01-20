package excel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExcelTools Excel工具实例
type ExcelTools struct {
	// baseDir 是允许操作的基础目录
	baseDir string
}

// NewExcelTools 创建一个新的Excel工具实例
// baseDir 是允许操作的基础目录
func NewExcelTools(baseDir string) (*ExcelTools, error) {
	// 转换为绝对路径
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	// 检查目录是否存在，如果不存在则创建
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
		}
	}

	return &ExcelTools{
		baseDir: absPath,
	}, nil
}

// validatePath 验证并规范化路径，确保路径在允许的目录范围内
func (et *ExcelTools) validatePath(userPath string) (string, error) {
	if userPath == "" {
		return et.baseDir, nil
	}

	// 清理路径
	cleanPath := filepath.Clean(userPath)

	// 如果是相对路径，则相对于 baseDir
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(et.baseDir, cleanPath)
	}

	// 转换为绝对路径以检查路径穿越
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("无法解析路径: %w", err)
	}

	// 检查路径是否在允许的目录内
	if !strings.HasPrefix(absPath, et.baseDir) {
		return "", fmt.Errorf("访问被拒绝: 路径 %s 不在允许的目录 %s 内", absPath, et.baseDir)
	}

	return absPath, nil
}

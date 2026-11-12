package excel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"
	"fkteams/internal/runtime/resources"

	"github.com/xuri/excelize/v2"
)

// ExcelTools 通过工作区路径和实例级资源锁访问工作簿。
type ExcelTools struct {
	baseDir string
	locks   *resources.KeyedLocks
}

// NewExcelTools 创建受工作区约束的 Excel 工具，可注入应用共享资源锁。
func NewExcelTools(baseDir string, shared ...*resources.KeyedLocks) (*ExcelTools, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	locks := resources.NewKeyedLocks()
	if len(shared) > 0 && shared[0] != nil {
		locks = shared[0]
	}
	return &ExcelTools{baseDir: abs, locks: locks}, nil
}

func (et *ExcelTools) validatePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("workbook path is required")
	}
	resolved, err := pathguard.ResolveWorkspace(et.baseDir, path)
	if err != nil {
		return "", fmt.Errorf("access denied: %w", err)
	}
	return resolved.AbsPath, nil
}

type workbook struct {
	*excelize.File
	unlock func()
}

func (f *workbook) Close() error { defer f.unlock(); return f.File.Close() }

func (et *ExcelTools) openWorkbook(ctx context.Context, path string, readOnly ...bool) (*workbook, error) {
	// 文件、Git 与工作簿工具共享工作区锁，防止跨工具的读改写互相覆盖。
	var unlock func()
	var err error
	if len(readOnly) > 0 && readOnly[0] {
		unlock, err = et.locks.AcquireRead(ctx, et.baseDir)
	} else {
		unlock, err = et.locks.Acquire(ctx, et.baseDir)
	}
	if err != nil {
		return nil, err
	}
	resolved, err := pathguard.ResolveWorkspace(et.baseDir, path)
	if err != nil {
		unlock()
		return nil, err
	}
	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		unlock()
		return nil, err
	}
	source, err := root.Open(resolved.RelPath)
	closeErr := root.Close()
	if err != nil {
		unlock()
		return nil, err
	}
	if closeErr != nil {
		source.Close()
		unlock()
		return nil, closeErr
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
		source.Close()
		unlock()
		return nil, fmt.Errorf("workbook exceeds file limits")
	}
	f, err := excelize.OpenReader(source, excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 8 << 20})
	closeErr = source.Close()
	if err != nil {
		unlock()
		return nil, err
	}
	if closeErr != nil {
		f.Close()
		unlock()
		return nil, closeErr
	}
	return &workbook{File: f, unlock: unlock}, nil
}

func (et *ExcelTools) saveWorkbook(f interface {
	WriteTo(io.Writer, ...excelize.Options) (int64, error)
}, path string) error {
	resolved, err := pathguard.ResolveWorkspace(et.baseDir, path)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		return err
	}
	parent := filepath.Dir(resolved.RelPath)
	if err := pathguard.EnsureRootDirectory(root, parent, 0755); err != nil {
		root.Close()
		return err
	}
	_, writeErr := atomicfile.WriteInRoot(root, resolved.RelPath, 0644, func(w io.Writer) (int64, error) { return f.WriteTo(w) })
	return errors.Join(writeErr, root.Close())
}

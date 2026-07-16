// Package pathguard 提供共享的工作区路径校验工具。
package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolvedPath 表示已校验的工作区内路径。
type ResolvedPath struct {
	BaseAbs string
	AbsPath string
	RelPath string
}

// ResolveWorkspace 将 userPath 解析到 baseDir 下，并校验已有路径或最近存在的父目录
// 不会通过符号链接逃逸出工作区。
func ResolveWorkspace(baseDir, userPath string) (ResolvedPath, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve workspace: %w", err)
	}
	baseAbs = filepath.Clean(baseAbs)

	cleanPath := filepath.Clean(userPath)
	var absPath string
	if userPath == "" || cleanPath == "." {
		absPath = baseAbs
	} else if filepath.IsAbs(cleanPath) {
		absPath = filepath.Clean(cleanPath)
	} else {
		absPath = filepath.Clean(filepath.Join(baseAbs, cleanPath))
	}

	if !isWithin(absPath, baseAbs) {
		return ResolvedPath{}, fmt.Errorf("path is outside workspace")
	}

	realBase, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve workspace symlink: %w", err)
	}
	realBase = filepath.Clean(realBase)

	existing := absPath
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return ResolvedPath{}, fmt.Errorf("stat path: %w", err)
		}
		if existing == baseAbs {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return ResolvedPath{}, fmt.Errorf("path is outside workspace")
		}
		existing = parent
	}

	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve path symlink: %w", err)
	}
	realExisting = filepath.Clean(realExisting)
	if !isWithin(realExisting, realBase) {
		return ResolvedPath{}, fmt.Errorf("path is outside workspace")
	}

	relPath, err := filepath.Rel(baseAbs, absPath)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve relative path: %w", err)
	}
	if relPath == "." {
		relPath = ""
	}
	if relPath != "" && strings.HasPrefix(relPath, "..") {
		return ResolvedPath{}, fmt.Errorf("path is outside workspace")
	}

	return ResolvedPath{
		BaseAbs: baseAbs,
		AbsPath: absPath,
		RelPath: relPath,
	}, nil
}

func isWithin(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	return path == base || strings.HasPrefix(path, base+string(os.PathSeparator))
}

// RejectRootSymlinkComponents 拒绝 root 内相对路径上的任意符号链接组件。
func RejectRootSymlinkComponents(root *os.Root, relativePath string) error {
	if root == nil {
		return fmt.Errorf("root is nil")
	}
	current := ""
	for _, component := range strings.Split(filepath.Clean(relativePath), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not allowed")
		}
	}
	return nil
}

// EnsureRootDirectory 在 root 内逐级创建目录，并拒绝已有的符号链接或非目录组件。
func EnsureRootDirectory(root *os.Root, relativePath string, perm os.FileMode) error {
	if root == nil {
		return fmt.Errorf("root is nil")
	}
	current := ""
	for _, component := range strings.Split(filepath.Clean(relativePath), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(current, perm); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("path component is not a regular directory")
		}
	}
	return nil
}

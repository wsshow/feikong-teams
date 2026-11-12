// Package workspace 将文件工具的实际系统调用限制在工作区根目录内。
package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"

	"github.com/spf13/afero"
)

// FS 实现 afero 文件系统，但每次文件操作都由 os.Root 约束实际解析路径。
type FS struct{ base string }

var _ afero.Fs = (*FS)(nil)

// New 创建绑定到已存在工作区的文件系统，不长期占用根目录句柄。
func New(base string) (*FS, error) {
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	if err := root.Close(); err != nil {
		return nil, err
	}
	return &FS{base: abs}, nil
}

func (f *FS) within(fn func(*os.Root) error) error {
	root, err := os.OpenRoot(f.base)
	if err != nil {
		return err
	}
	err = fn(root)
	return errors.Join(err, root.Close())
}
func local(name string) string {
	if name == "" {
		return "."
	}
	return name
}

// Name 返回文件系统名称。
func (f *FS) Name() string { return "workspace-root" }

// Create 在根目录内创建文件。
func (f *FS) Create(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// Open 打开根目录内的文件。
func (f *FS) Open(name string) (afero.File, error) { return f.OpenFile(name, os.O_RDONLY, 0) }

// OpenFile 使用指定标志打开根目录内的文件。
func (f *FS) OpenFile(name string, flag int, mode os.FileMode) (afero.File, error) {
	var file *os.File
	err := f.within(func(root *os.Root) error {
		var err error
		file, err = root.OpenFile(local(name), flag, mode)
		return err
	})
	if err != nil {
		if file != nil {
			file.Close()
		}
		return nil, err
	}
	return file, nil
}

// Mkdir 创建目录。
func (f *FS) Mkdir(name string, mode os.FileMode) error {
	return f.within(func(r *os.Root) error { return r.Mkdir(local(name), mode) })
}

// MkdirAll 创建根目录内的父目录链。
func (f *FS) MkdirAll(name string, mode os.FileMode) error {
	return f.within(func(r *os.Root) error { return r.MkdirAll(local(name), mode) })
}

// Remove 删除根目录内的文件或空目录。
func (f *FS) Remove(name string) error {
	return f.within(func(r *os.Root) error { return r.Remove(local(name)) })
}

// RemoveAll 删除根目录内的目录树。
func (f *FS) RemoveAll(name string) error {
	return f.within(func(r *os.Root) error { return r.RemoveAll(local(name)) })
}

// Rename 在同一根目录内重命名。
func (f *FS) Rename(from, to string) error {
	return f.within(func(r *os.Root) error { return r.Rename(local(from), local(to)) })
}

// Stat 读取根目录内文件信息。
func (f *FS) Stat(name string) (os.FileInfo, error) {
	var info os.FileInfo
	err := f.within(func(r *os.Root) error { var err error; info, err = r.Stat(local(name)); return err })
	return info, err
}

// LstatIfPossible 读取链接本身的信息，遍历不会跟随目录链接。
func (f *FS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	var info os.FileInfo
	err := f.within(func(r *os.Root) error { var err error; info, err = r.Lstat(local(name)); return err })
	return info, true, err
}

// Chmod 修改根目录内文件权限。
func (f *FS) Chmod(name string, mode os.FileMode) error {
	return f.within(func(r *os.Root) error { return r.Chmod(local(name), mode) })
}

// Chown 修改根目录内文件所有者。
func (f *FS) Chown(name string, uid, gid int) error {
	return f.within(func(r *os.Root) error { return r.Chown(local(name), uid, gid) })
}

// Chtimes 修改根目录内文件时间。
func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	return f.within(func(r *os.Root) error { return r.Chtimes(local(name), atime, mtime) })
}

// WriteAtomic 在根目录内完整写入并原子替换，避免失败写入破坏原文件。
func (f *FS) WriteAtomic(name string, data []byte, mode os.FileMode) error {
	return f.within(func(r *os.Root) error {
		if err := pathguard.EnsureRootDirectory(r, filepath.Dir(name), 0755); err != nil {
			return err
		}
		if info, err := r.Stat(name); err == nil {
			mode = info.Mode().Perm()
		}
		return atomicfile.WriteFileInRoot(r, name, data, mode)
	})
}

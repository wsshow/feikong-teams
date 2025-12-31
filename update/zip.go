package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// UnzipCallback 定义进度回调函数
type UnzipCallback func(processed int, total int, fileName string, isDir bool)

// Unzip 带进度回调的解压函数
func Unzip(source, destination string, callback UnzipCallback) error {
	// 1. 打开 zip 文件
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()

	// 2. 确保目标目录存在
	destDir := filepath.Clean(destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	totalFiles := len(reader.File)

	// 3. 遍历文件
	for i, f := range reader.File {
		// 触发进度回调
		if callback != nil {
			callback(i+1, totalFiles, f.Name, f.FileInfo().IsDir())
		}

		// 将单个文件的处理逻辑抽取出来，便于使用 defer 管理资源
		err := extractFile(f, destDir)
		if err != nil {
			return fmt.Errorf("解压文件 %s 失败: %w", f.Name, err)
		}
	}
	return nil
}

// extractFile 处理单个文件的解压逻辑
func extractFile(f *zip.File, destDir string) error {
	// 拼接路径
	fpath := filepath.Join(destDir, f.Name)

	// --- 安全检查 (Zip Slip) ---
	// 必须校验 fpath 是否真的在 destDir 目录下
	if !strings.HasPrefix(fpath, destDir+string(os.PathSeparator)) {
		return fmt.Errorf("非法路径 (Zip Slip): %s", fpath)
	}

	// 如果是目录，直接创建
	if f.FileInfo().IsDir() {
		return os.MkdirAll(fpath, os.ModePerm)
	}

	// 确保父目录存在 (防止 Zip 中文件顺序乱序，即文件在目录之前出现)
	if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
		return err
	}

	// --- 打开源文件 ---
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	// --- 修正文件权限 ---
	// 如果 zip 来自不同系统，Mode 可能会有问题。
	// 这里通过位运算确保当前用户至少有读写权限 (0600)，防止解压出无法操作的死文件
	mode := f.Mode()
	if mode&0200 == 0 { // 如果没有写权限
		mode |= 0200 // 强制加上写权限
	}

	// --- 处理软链接 ---
	if f.Mode()&os.ModeSymlink != 0 {
		return nil // 跳过软链接处理
	}

	// --- 创建目标文件 ---
	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// --- 写入数据 ---
	// 为了防止大文件解压占用过多内存，io.Copy 是正确的选择
	_, err = io.Copy(outFile, rc)
	return err
}

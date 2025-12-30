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
// processed: 已处理的文件数
// total: 总文件数
// fileName: 当前正在处理的文件名
// isDir: 是否是目录
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
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}

	totalFiles := len(reader.File)

	// 3. 遍历文件
	for i, f := range reader.File {
		// 触发进度回调
		if callback != nil {
			callback(i+1, totalFiles, f.Name, f.FileInfo().IsDir())
		}

		fpath := filepath.Join(destination, f.Name)

		// 安全检查 (Zip Slip)
		if !strings.HasPrefix(fpath, filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: 非法的解压路径", fpath)
		}

		// 如果是目录
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// 创建文件所在的父目录
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		// 打开并创建文件
		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

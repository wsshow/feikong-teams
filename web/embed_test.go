package web

import (
	"io/fs"
	"os"
	"testing"
)

func TestGetFSIncludesAllBuildFiles(t *testing.T) {
	embedded := GetFS()
	// 从磁盘枚举产物，避免嵌入规则漏掉的文件也被测试忽略。
	err := fs.WalkDir(os.DirFS("dist"), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if _, err := fs.ReadFile(embedded, name); err != nil {
			t.Errorf("build file %q is not readable from embedded filesystem: %v", name, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk build directory: %v", err)
	}
}

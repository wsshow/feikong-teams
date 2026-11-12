package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootFilesystemRejectsEscapesAtAccessTime(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	fs, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	// 链接在构造后出现，验证实际 open 仍受根目录约束。
	if err := os.Symlink(target, filepath.Join(base, "link")); err != nil {
		t.Skip(err)
	}
	for _, name := range []string{"link", "../secret.txt", target} {
		if file, err := fs.Open(name); err == nil {
			file.Close()
			t.Fatalf("escape opened: %s", name)
		}
	}
	if err := os.Symlink(outside, filepath.Join(base, "directory")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteAtomic("directory/new.txt", []byte("blocked"), 0600); err == nil {
		t.Fatal("write escaped workspace")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePathResolvesRelativePath(t *testing.T) {
	ft, err := NewFileTools(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	got, err := ft.workspacePath(filepath.Join("dir", "file.txt"))
	if err != nil {
		t.Fatalf("workspacePath returned error: %v", err)
	}
	if got != filepath.Join("dir", "file.txt") {
		t.Fatalf("unexpected relative path: %q", got)
	}
}

func TestWorkspacePathRejectsOutsideSymlink(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "link.txt")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	ft, err := NewFileTools(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ft.workspacePath("link.txt"); err == nil {
		t.Fatal("expected symlink to outside file to be rejected")
	}
}

func TestWorkspacePathRejectsNewFileUnderOutsideSymlinkDir(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "out")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	ft, err := NewFileTools(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ft.workspacePath(filepath.Join("out", "new.txt")); err == nil {
		t.Fatal("expected new file below outside symlink dir to be rejected")
	}
}

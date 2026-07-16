package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspaceRelativePath(t *testing.T) {
	base := t.TempDir()
	got, err := ResolveWorkspace(base, "dir/file.txt")
	if err != nil {
		t.Fatalf("resolve workspace path: %v", err)
	}
	if got.RelPath != filepath.Join("dir", "file.txt") {
		t.Fatalf("unexpected rel path: %q", got.RelPath)
	}
	if got.AbsPath != filepath.Join(base, "dir", "file.txt") {
		t.Fatalf("unexpected abs path: %q", got.AbsPath)
	}
}

func TestResolveWorkspaceRejectsParentEscape(t *testing.T) {
	base := t.TempDir()
	if _, err := ResolveWorkspace(base, "../outside.txt"); err == nil {
		t.Fatal("expected parent escape to be rejected")
	}
}

func TestResolveWorkspaceRejectsSymlinkToOutsideFile(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "link.txt")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if _, err := ResolveWorkspace(base, "link.txt"); err == nil {
		t.Fatal("expected symlink to outside file to be rejected")
	}
}

func TestResolveWorkspaceRejectsNewFileUnderOutsideSymlinkDir(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "out")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if _, err := ResolveWorkspace(base, filepath.Join("out", "new.txt")); err == nil {
		t.Fatal("expected new file below outside symlink dir to be rejected")
	}
}

func TestRejectRootSymlinkComponents(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "linked")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := RejectRootSymlinkComponents(root, filepath.Join("real")); err != nil {
		t.Fatalf("regular path rejected: %v", err)
	}
	if err := RejectRootSymlinkComponents(root, filepath.Join("linked")); err == nil {
		t.Fatal("symlink path should be rejected")
	}
	if err := EnsureRootDirectory(root, filepath.Join("linked", "nested"), 0755); err == nil {
		t.Fatal("directory creation through symlink should be rejected")
	}
	if _, err := os.Stat(filepath.Join(base, "real", "nested")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory was created through symlink: %v", err)
	}
	if err := EnsureRootDirectory(root, filepath.Join("created", "nested"), 0755); err != nil {
		t.Fatalf("create regular directories: %v", err)
	}
}

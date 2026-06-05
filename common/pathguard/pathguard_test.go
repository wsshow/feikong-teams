package pathguard

import (
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

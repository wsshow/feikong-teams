package utils

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPathExists(t *testing.T) {
	dir := t.TempDir()
	exists, err := PathExists(dir)
	if err != nil {
		t.Fatalf("PathExists(existing) error = %v", err)
	}
	if !exists {
		t.Fatal("PathExists(existing) = false, want true")
	}

	exists, err = PathExists(filepath.Join(dir, "missing"))
	if err != nil {
		t.Fatalf("PathExists(missing) error = %v", err)
	}
	if exists {
		t.Fatal("PathExists(missing) = true, want false")
	}
}

func TestEnsureDirCreatesAndAcceptsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir(create) error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s should be a directory", dir)
	}
	if err := NotExistToMkdir(dir); err != nil {
		t.Fatalf("NotExistToMkdir(existing) error = %v", err)
	}
}

func TestEnsureDirRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := EnsureDir(path)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("EnsureDir(file) error = %v, want os.ErrExist", err)
	}
}

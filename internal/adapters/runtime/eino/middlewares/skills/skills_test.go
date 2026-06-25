package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	einoruntime "fkteams/internal/adapters/runtime/eino"
)

func TestEnsureDirCreatesAndAcceptsExistingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	if err := ensureDir(dir); err != nil {
		t.Fatalf("ensureDir(create) error = %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s should be a directory", dir)
	}

	if err := ensureDir(dir); err != nil {
		t.Fatalf("ensureDir(existing) error = %v", err)
	}
}

func TestEnsureDirReturnsErrorForFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills")
	if err := os.WriteFile(path, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := ensureDir(path)
	if err == nil {
		t.Fatal("ensureDir(file) error = nil, want error")
	}
}

func TestNewCreatesSkillsMiddlewareUnderAppDir(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", appDir)

	middleware, err := New(context.Background())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if middleware == nil {
		t.Fatal("New() returned nil middleware")
	}
	if middleware.Name() != "skills" {
		t.Fatalf("middleware name = %q, want skills", middleware.Name())
	}
	if _, err := einoruntime.AdaptAgentMiddlewareForRunner(middleware); err != nil {
		t.Fatalf("adapt middleware: %v", err)
	}

	skillsDir := filepath.Join(appDir, "skills")
	info, err := os.Stat(skillsDir)
	if err != nil {
		t.Fatalf("stat skills dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s should be a directory", skillsDir)
	}
}

func TestNewReportsInaccessibleSkillsDirectory(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", appDir)
	if err := os.WriteFile(filepath.Join(appDir, "skills"), []byte("not a directory"), 0644); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}

	_, err := New(context.Background())
	if err == nil {
		t.Fatal("New() error = nil, want skills directory error")
	}
	if !strings.Contains(err.Error(), "无法创建或访问目录") {
		t.Fatalf("New() error = %v, want directory access error", err)
	}
}

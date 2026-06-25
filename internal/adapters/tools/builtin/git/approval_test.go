package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fkteams/internal/app/tools/approval"
)

func TestGitInitRequiresApproval(t *testing.T) {
	baseDir := t.TempDir()
	gt, err := NewGitTools(baseDir)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := gt.GitInit(context.Background(), &GitInitRequest{})
	if err == nil {
		t.Fatalf("expected interrupt error, got response: %#v", resp)
	}
	if _, statErr := os.Stat(filepath.Join(baseDir, ".git")); !os.IsNotExist(statErr) {
		t.Fatalf("repository should not be initialized before approval: %v", statErr)
	}
}

func TestGitInitAllowsAutoApprovedGitStore(t *testing.T) {
	baseDir := t.TempDir()
	gt, err := NewGitTools(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := approval.WithRegistry(context.Background(), approval.NewAutoApproveRegistry())

	resp, err := gt.GitInit(ctx, &GitInitRequest{})
	if err != nil {
		t.Fatalf("git init: %v", err)
	}
	if resp.ErrorMessage != "" {
		t.Fatalf("unexpected response error: %s", resp.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(baseDir, ".git")); err != nil {
		t.Fatalf("repository should be initialized: %v", err)
	}
}

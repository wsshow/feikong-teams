package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestBranchNamesAndExistingReferencesAreProtected(t *testing.T) {
	gt, path, ctx := newInitializedGitRepo(t)
	if err := os.WriteFile(filepath.Join(path, "file"), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if resp, err := gt.GitAdd(ctx, &GitAddRequest{Files: []string{"file"}}); err != nil || resp.ErrorMessage != "" {
		t.Fatal(resp, err)
	}
	first, err := gt.GitCommit(ctx, &GitCommitRequest{Message: "first"})
	if err != nil || first.ErrorMessage != "" {
		t.Fatal(first, err)
	}
	created, err := gt.GitBranch(ctx, &GitBranchRequest{Action: "create", Name: "exists"})
	if err != nil || created.ErrorMessage != "" {
		t.Fatal(created, err)
	}
	before, _ := os.ReadFile(filepath.Join(path, ".git", "config"))
	for _, name := range []string{"../../config", "../escape", "bad..name", "exists"} {
		resp, err := gt.GitBranch(ctx, &GitBranchRequest{Action: "create", Name: name})
		if err != nil || resp.ErrorMessage == "" {
			t.Fatalf("accepted invalid branch %q: %#v %v", name, resp, err)
		}
	}
	after, _ := os.ReadFile(filepath.Join(path, ".git", "config"))
	if string(before) != string(after) {
		t.Fatal("branch creation modified config")
	}
	repo, err := gt.openRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName("exists"), false)
	if err != nil || ref.Hash().String() != first.CommitHash {
		t.Fatal("existing branch was changed")
	}
}
func TestGitRejectsOutsideWorkspaceAndMetadataLinks(t *testing.T) {
	base, outside := t.TempDir(), t.TempDir()
	gt, err := NewGitTools(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "linked")); err != nil {
		t.Skip(err)
	}
	if _, err := gt.validatePath("linked"); err == nil {
		t.Fatal("outside repository link accepted")
	}
	repo := filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := gt.validatePath("repo"); err == nil {
		t.Fatal("outside metadata link accepted")
	}
	if err := os.Remove(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+outside), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := gt.validatePath("repo"); err == nil {
		t.Fatal("outside gitdir file accepted")
	}
}
func TestGitDiffUsesIndexForStagedAndUnstagedChanges(t *testing.T) {
	gt, path, ctx := newInitializedGitRepo(t)
	file := filepath.Join(path, "text.txt")
	os.WriteFile(file, []byte("original\n"), 0600)
	gt.GitAdd(ctx, &GitAddRequest{Files: []string{"text.txt"}})
	gt.GitCommit(ctx, &GitCommitRequest{Message: "initial"})
	os.WriteFile(file, []byte("staged\n"), 0600)
	gt.GitAdd(ctx, &GitAddRequest{Files: []string{"text.txt"}})
	os.WriteFile(file, []byte("working\n"), 0600)
	for _, cached := range []bool{false, true} {
		resp, err := gt.GitDiff(context.Background(), &GitDiffRequest{Cached: cached})
		if err != nil || resp.ErrorMessage != "" || len(resp.Diffs) != 1 {
			t.Fatalf("diff: %#v %v", resp, err)
		}
		item := resp.Diffs[0]
		old, new := "staged", "working"
		if cached {
			old, new = "original", "staged"
		}
		if item.Additions != 1 || item.Deletions != 1 || !strings.Contains(item.Patch, "-"+old) || !strings.Contains(item.Patch, "+"+new) {
			t.Fatalf("wrong diff: %#v", item)
		}
	}
}

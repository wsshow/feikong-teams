package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fkteams/internal/app/tools/approval"
)

func TestValidatePathRejectsSiblingPrefix(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "repo")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(parent, "repo-sibling")
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatal(err)
	}

	gt, err := NewGitTools(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gt.validatePath("."); err != nil {
		t.Fatalf("validate inside path: %v", err)
	}
	if _, err := gt.validatePath(sibling); err == nil || !strings.Contains(err.Error(), "访问被拒绝") {
		t.Fatalf("sibling path error = %v, want denied", err)
	}
}

func TestGitToolsBasicRepositoryWorkflow(t *testing.T) {
	gt, repoPath, ctx := newInitializedGitRepo(t)

	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	status, err := gt.GitStatus(context.Background(), &GitStatusRequest{})
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	if status.IsClean || len(status.Files) != 1 || status.Files[0].Worktree != "未跟踪" {
		t.Fatalf("status = %#v, want one untracked file", status)
	}

	addResp, err := gt.GitAdd(ctx, &GitAddRequest{Files: []string{"README.md"}})
	if err != nil {
		t.Fatalf("GitAdd: %v", err)
	}
	if addResp.ErrorMessage != "" || len(addResp.AddedFiles) != 1 {
		t.Fatalf("add response = %#v", addResp)
	}

	diff, err := gt.GitDiff(context.Background(), &GitDiffRequest{Cached: true})
	if err != nil {
		t.Fatalf("GitDiff: %v", err)
	}
	if len(diff.Diffs) != 1 || diff.Diffs[0].Status != "已添加" {
		t.Fatalf("cached diff = %#v, want added README", diff)
	}

	commitResp, err := gt.GitCommit(ctx, &GitCommitRequest{
		Message: "initial commit",
		Author:  "Tester",
		Email:   "tester@example.com",
	})
	if err != nil {
		t.Fatalf("GitCommit: %v", err)
	}
	if commitResp.ErrorMessage != "" || commitResp.CommitHash == "" {
		t.Fatalf("commit response = %#v", commitResp)
	}

	logResp, err := gt.GitLog(context.Background(), &GitLogRequest{Limit: 1})
	if err != nil {
		t.Fatalf("GitLog: %v", err)
	}
	if len(logResp.Commits) != 1 || logResp.Commits[0].Message != "initial commit" || logResp.Commits[0].Author != "Tester" {
		t.Fatalf("log response = %#v", logResp)
	}

	createBranch, err := gt.GitBranch(ctx, &GitBranchRequest{Action: "create", Name: "feature"})
	if err != nil {
		t.Fatalf("GitBranch create: %v", err)
	}
	if createBranch.ErrorMessage != "" {
		t.Fatalf("create branch response = %#v", createBranch)
	}
	branches, err := gt.GitBranch(context.Background(), &GitBranchRequest{Action: "list"})
	if err != nil {
		t.Fatalf("GitBranch list: %v", err)
	}
	if !hasBranch(branches.Branches, "feature") {
		t.Fatalf("branches = %#v, want feature", branches.Branches)
	}
}

func TestGitConfigSetGetAndList(t *testing.T) {
	gt, _, ctx := newInitializedGitRepo(t)

	setResp, err := gt.GitConfig(ctx, &GitConfigRequest{Action: "set", Key: "user.name", Value: "Tester"})
	if err != nil {
		t.Fatalf("GitConfig set: %v", err)
	}
	if setResp.ErrorMessage != "" {
		t.Fatalf("set response = %#v", setResp)
	}

	getResp, err := gt.GitConfig(context.Background(), &GitConfigRequest{Action: "get", Key: "user.name"})
	if err != nil {
		t.Fatalf("GitConfig get: %v", err)
	}
	if getResp.Value != "Tester" {
		t.Fatalf("get value = %q, want Tester", getResp.Value)
	}

	listResp, err := gt.GitConfig(context.Background(), &GitConfigRequest{Action: "list"})
	if err != nil {
		t.Fatalf("GitConfig list: %v", err)
	}
	if !hasConfig(listResp.Configs, "user.name", "Tester") {
		t.Fatalf("configs = %#v, want user.name", listResp.Configs)
	}
}

func TestGetToolsReturnsGitToolSet(t *testing.T) {
	gt, err := NewGitTools(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tools, err := gt.GetTools()
	if err != nil {
		t.Fatalf("GetTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info: %v", err)
		}
		names[info.Name] = true
	}
	for _, name := range []string{"git_init", "git_status", "git_add", "git_commit", "git_log", "git_branch", "git_diff", "git_config"} {
		if !names[name] {
			t.Fatalf("tool %s missing from %#v", name, names)
		}
	}
}

func newInitializedGitRepo(t *testing.T) (*GitTools, string, context.Context) {
	t.Helper()
	repoPath := t.TempDir()
	gt, err := NewGitTools(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := approval.WithRegistry(context.Background(), approval.NewAutoApproveRegistry())
	resp, err := gt.GitInit(ctx, &GitInitRequest{})
	if err != nil {
		t.Fatalf("GitInit: %v", err)
	}
	if resp.ErrorMessage != "" {
		t.Fatalf("GitInit response = %#v", resp)
	}
	return gt, repoPath, ctx
}

func hasBranch(branches []BranchInfo, name string) bool {
	for _, branch := range branches {
		if branch.Name == name {
			return true
		}
	}
	return false
}

func hasConfig(configs []ConfigItem, key, value string) bool {
	for _, item := range configs {
		if item.Key == key && item.Value == value {
			return true
		}
	}
	return false
}

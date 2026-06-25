package approval

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestOperationInfoFormatsReusablePrompt(t *testing.T) {
	op := Operation{
		Title:  "Git operation requires approval",
		Target: "/tmp/repo",
		Details: []OperationDetail{
			{Name: "Operation", Value: "commit"},
			{Name: "Secret", Value: ""},
		},
	}

	got := op.Info()
	want := "Git operation requires approval\n  Target: /tmp/repo\n  Operation: commit"
	if got != want {
		t.Fatalf("unexpected operation info:\n%s", got)
	}
}

func TestOperationInfoUsesDefaultTitle(t *testing.T) {
	op := Operation{}
	if got := op.Info(); got != "Operation requires approval" {
		t.Fatalf("unexpected default info: %q", got)
	}
}

func TestRejectedMessage(t *testing.T) {
	got, ok := RejectedMessage(ErrRejected, "custom rejected")
	if !ok {
		t.Fatal("expected rejection to be detected")
	}
	if got != "custom rejected" {
		t.Fatalf("unexpected rejected message: %q", got)
	}

	if _, ok := RejectedMessage(errors.New("other"), "custom rejected"); ok {
		t.Fatal("unexpected rejection match")
	}
}

func TestNewDefaultRegistryUsesSharedStores(t *testing.T) {
	reg := NewDefaultRegistry()
	for _, name := range []string{StoreCommand, StoreFile, StoreGit, StoreDispatch} {
		if reg.get(name) == nil {
			t.Fatalf("expected store %q", name)
		}
	}

	fileStore := reg.get(StoreFile)
	fileStore.approve(filepath.Join("workspace", "project"))
	if !fileStore.IsApproved(filepath.Join("workspace", "project", "notes.md")) {
		t.Fatal("expected file store to approve child paths")
	}

	gitStore := reg.get(StoreGit)
	gitStore.approve(filepath.Join("workspace", "repo"))
	if !gitStore.IsApproved(filepath.Join("workspace", "repo", ".git")) {
		t.Fatal("expected git store to approve child paths")
	}
}

func TestNewDefaultSelectiveRegistryApprovesRequestedStores(t *testing.T) {
	reg := NewDefaultSelectiveRegistry([]string{StoreCommand})

	if !reg.get(StoreCommand).IsApproved("any") {
		t.Fatal("expected command store to be auto approved")
	}
	if reg.get(StoreDispatch).IsApproved("any") {
		t.Fatal("did not expect dispatch store to be auto approved")
	}
}

func TestRegistryContextInjectsRegistry(t *testing.T) {
	reg := NewAutoApproveRegistry()
	ctx := RegistryContext(reg)(context.Background())

	if err := Require(ctx, StoreCommand, "command", "info"); err != nil {
		t.Fatalf("expected injected registry to approve command: %v", err)
	}
}

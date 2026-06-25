package tools

import (
	"context"
	"testing"

	apptools "fkteams/internal/app/tools"
)

func TestBootstrapRegistersSchedulerToolGroup(t *testing.T) {
	if err := RegisterDefaults(); err != nil {
		t.Fatalf("RegisterDefaults should be idempotent: %v", err)
	}
	if !contains(apptools.BuiltinToolNames(), "scheduler") {
		t.Fatal("scheduler tool group is not registered")
	}
	resolved, err := apptools.GetToolsByName("scheduler")
	if err != nil {
		t.Fatalf("GetToolsByName returned error: %v", err)
	}
	if len(resolved) == 0 {
		t.Fatal("expected scheduler tools")
	}
	info, err := resolved[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info: %v", err)
	}
	if info.Name != "schedule_add" {
		t.Fatalf("first scheduler tool = %q, want schedule_add", info.Name)
	}
}

func TestBootstrapRegistersGitToolGroup(t *testing.T) {
	if err := RegisterDefaults(); err != nil {
		t.Fatalf("RegisterDefaults should be idempotent: %v", err)
	}
	if !contains(apptools.BuiltinToolNames(), "git") {
		t.Fatal("git tool group is not registered")
	}
	resolved, err := apptools.GetToolsByName("git")
	if err != nil {
		t.Fatalf("GetToolsByName returned error: %v", err)
	}
	if len(resolved) == 0 {
		t.Fatal("expected git tools")
	}
	info, err := resolved[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info: %v", err)
	}
	if info.Name != "git_init" {
		t.Fatalf("first git tool = %q, want git_init", info.Name)
	}
}

func TestBootstrapRegistersSSHToolGroup(t *testing.T) {
	if err := RegisterDefaults(); err != nil {
		t.Fatalf("RegisterDefaults should be idempotent: %v", err)
	}
	infos := apptools.BuiltinToolInfos()
	for _, info := range infos {
		if info.Name != "ssh" {
			continue
		}
		if info.DisplayName == "" || info.Category == "" || len(info.IncludedTools) == 0 {
			t.Fatalf("ssh tool info is incomplete: %#v", info)
		}
		return
	}
	t.Fatal("ssh tool group is not registered")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

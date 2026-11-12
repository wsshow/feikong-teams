package tools

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	mcpadapter "fkteams/internal/adapters/tools/mcp"
	"fkteams/internal/app/config"
	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
)

func TestBootstrapRegistersSchedulerToolGroup(t *testing.T) {
	registry, err := RegisterDefaults(mcpadapter.NewProvider())
	if err != nil {
		t.Fatalf("RegisterDefaults should be idempotent: %v", err)
	}
	ctx := apptools.WithRegistry(context.Background(), registry)
	if !contains(apptools.BuiltinToolNames(ctx), "scheduler") {
		t.Fatal("scheduler tool group is not registered")
	}
	resolved, err := apptools.GetToolsByName(ctx, "scheduler")
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
	registry, err := RegisterDefaults(mcpadapter.NewProvider())
	if err != nil {
		t.Fatalf("RegisterDefaults should be idempotent: %v", err)
	}
	ctx := apptools.WithRegistry(context.Background(), registry)
	if !contains(apptools.BuiltinToolNames(ctx), "git") {
		t.Fatal("git tool group is not registered")
	}
	resolved, err := apptools.GetToolsByName(ctx, "git")
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
	registry, err := RegisterDefaults(mcpadapter.NewProvider())
	if err != nil {
		t.Fatalf("RegisterDefaults should be idempotent: %v", err)
	}
	ctx := apptools.WithRegistry(context.Background(), registry)
	infos := apptools.BuiltinToolInfos(ctx)
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

func TestBootstrapRegistersJavaScriptToolGroup(t *testing.T) {
	registry, err := RegisterDefaults(mcpadapter.NewProvider())
	if err != nil {
		t.Fatalf("RegisterDefaults() error = %v", err)
	}
	if _, ok, err := registry.Resolve(context.Background(), "javascript", nil); err != nil || !ok {
		t.Fatalf("Resolve(javascript) = ok %v, error %v", ok, err)
	}
	for _, info := range registry.Infos() {
		if info.Name != "javascript" {
			continue
		}
		if info.DisplayName != "自定义工具" || strings.Contains(strings.ToLower(info.Description), "goja") {
			t.Fatalf("javascript tool group user-facing metadata = %#v", info)
		}
		return
	}
	t.Fatal("javascript tool group info is not registered")
}

func TestJavaScriptToolGroupUsesResolveContextConfig(t *testing.T) {
	registry, err := RegisterDefaults(mcpadapter.NewProvider())
	if err != nil {
		t.Fatalf("RegisterDefaults() error = %v", err)
	}
	cfg := &config.Config{JavaScript: config.JavaScriptSettings{Tools: []config.JavaScriptTool{{
		ID: "preview", Name: "预览", Description: "测试配置覆盖", Enabled: true,
		Source: `function execute(input) { return input; }`,
	}}}}
	ctx := apptools.WithResolveContextPatch(context.Background(), apptools.ToolResolveContext{Config: cfg})
	resolved, err := registry.GetToolsByName(ctx, "javascript")
	if err != nil {
		t.Fatalf("GetToolsByName() error = %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("tool count = %d, want 1", len(resolved))
	}
	result, err := resolved[0].Invoke(ctx, runtimeport.ToolInvocation{Name: "preview", Arguments: `{"value":1}`})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Content != `{"value":1}` {
		t.Fatalf("Invoke() content = %s", result.Content)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestToolCatalogMatchesResolvedBuiltinNames(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", root)
	// 工具声明的校验不依赖本机安装或启动脚本运行时。
	bin := t.TempDir()
	for _, name := range []string{"bun", "uv"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	registry, err := RegisterDefaults(mcpadapter.NewProvider())
	if err != nil {
		t.Fatal(err)
	}
	ctx := apptools.WithResolveContextPatch(context.Background(), apptools.ToolResolveContext{WorkspaceDir: filepath.Join(root, "workspace"), SessionsDir: filepath.Join(root, "sessions"), RuntimeDir: filepath.Join(root, "runtime")})
	for _, group := range registry.Infos() {
		if !group.Builtin || group.Name == "ssh" {
			continue
		}
		tools, err := registry.GetToolsByName(ctx, group.Name)
		if err != nil {
			t.Fatalf("resolve %s: %v", group.Name, err)
		}
		actual := make([]string, 0, len(tools))
		for _, tool := range tools {
			info, err := tool.Info(ctx)
			if err != nil {
				t.Fatal(err)
			}
			actual = append(actual, info.Name)
		}
		expected := append([]string(nil), group.IncludedTools...)
		sort.Strings(expected)
		sort.Strings(actual)
		if !reflect.DeepEqual(expected, actual) {
			t.Fatalf("catalog %s differs: declared=%v actual=%v", group.Name, expected, actual)
		}
	}
}

package tools

import (
	"context"
	"fkteams/agentcore"
	"fkteams/tools/approval"
	"fkteams/tools/search"
	"testing"
)

type stubTool struct {
	info *agentcore.ToolInfo
}

func (t stubTool) Info(context.Context) (*agentcore.ToolInfo, error) {
	return t.info, nil
}

func (t stubTool) Invoke(context.Context, agentcore.ToolInvocation) (*agentcore.ToolResult, error) {
	return nil, nil
}

func mustPolicy(t *testing.T, name string) ToolPolicy {
	t.Helper()
	policy, ok := PolicyForTool(name)
	if !ok {
		t.Fatalf("missing policy for %s", name)
	}
	return policy
}

func TestGitToolPolicy(t *testing.T) {
	readOnly := []string{"git_status", "git_log", "git_diff"}
	for _, name := range readOnly {
		policy := mustPolicy(t, name)
		if !policy.ReadOnly {
			t.Fatalf("%s should be read-only", name)
		}
		if policy.Destructive {
			t.Fatalf("%s should not be destructive", name)
		}
	}

	destructive := []string{
		"git_init", "git_add", "git_commit", "git_checkout", "git_reset",
		"git_remove", "git_branch", "git_tag", "git_remote", "git_config", "git_clean",
	}
	for _, name := range destructive {
		policy := mustPolicy(t, name)
		if policy.ReadOnly {
			t.Fatalf("%s should not be read-only", name)
		}
		if !policy.Destructive {
			t.Fatalf("%s should be destructive", name)
		}
	}
}

func TestActualScriptToolNamesUsePolicy(t *testing.T) {
	destructive := []string{
		"bun_init_env", "bun_install_package", "bun_remove_package", "bun_clean_env", "bun_run_script",
		"uv_init_env", "uv_install_package", "uv_remove_package", "uv_clean_env", "uv_run_script", "uv_run_code", "uv_format_code",
	}
	for _, name := range destructive {
		policy := mustPolicy(t, name)
		if !policy.Destructive {
			t.Fatalf("%s should be destructive", name)
		}
		if !policy.Serialize {
			t.Fatalf("%s should be serialized", name)
		}
	}

	readOnly := []string{"bun_list_package", "bun_get_env_info", "uv_list_package", "uv_get_env_info", "uv_check_syntax"}
	for _, name := range readOnly {
		policy := mustPolicy(t, name)
		if !policy.ReadOnly {
			t.Fatalf("%s should be read-only", name)
		}
		if policy.Destructive {
			t.Fatalf("%s should not be destructive", name)
		}
	}
}

func TestDocumentToolNamesUsePolicy(t *testing.T) {
	for _, name := range []string{"get_document_info", "read_document_smart", "read_document_by_pages", "read_document_by_lines"} {
		if !mustPolicy(t, name).ReadOnly {
			t.Fatalf("%s should be read-only", name)
		}
	}
	if _, ok := PolicyForTool("doc_read"); ok {
		t.Fatal("legacy doc_read should not be classified")
	}
}

func TestSearchAndAskToolNamesUsePolicy(t *testing.T) {
	for _, name := range []string{"search", "fetch", "ask_questions"} {
		policy := mustPolicy(t, name)
		if !policy.ReadOnly {
			t.Fatalf("%s should be read-only", name)
		}
		if policy.Destructive {
			t.Fatalf("%s should not be destructive", name)
		}
	}
	if _, ok := PolicyForTool("duckduckgo_text_search"); ok {
		t.Fatal("unregistered search constructor default should not be classified as a builtin policy")
	}
}

func TestSearchPolicyMatchesActualBuiltinToolName(t *testing.T) {
	searchTools, err := search.GetTools()
	if err != nil {
		t.Fatalf("get search tools: %v", err)
	}
	if len(searchTools) != 1 {
		t.Fatalf("expected one search tool, got %d", len(searchTools))
	}
	info, err := searchTools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("get search tool info: %v", err)
	}
	if info.Name != "search" {
		t.Fatalf("unexpected search tool name: %s", info.Name)
	}
	if _, ok := PolicyForTool(info.Name); !ok {
		t.Fatalf("missing policy for actual search tool name: %s", info.Name)
	}
}

func TestPolicyIncludesApprovalAndExternalPath(t *testing.T) {
	filePolicy := mustPolicy(t, "file_read")
	if got := filePolicy.ApprovalStore; got != approval.StoreFile {
		t.Fatalf("unexpected file approval store: %q", got)
	}
	if !filePolicy.ExternalPath {
		t.Fatal("expected file_read to allow external path via approval")
	}
	if got := mustPolicy(t, "git_commit").ApprovalStore; got != approval.StoreGit {
		t.Fatalf("unexpected git approval store: %q", got)
	}
	if got := mustPolicy(t, "execute").ApprovalStore; got != approval.StoreCommand {
		t.Fatalf("unexpected command approval store: %q", got)
	}
}

func TestClassifyToolSetsMetadata(t *testing.T) {
	readTool := stubTool{info: &agentcore.ToolInfo{Name: "git_status"}}
	if err := ClassifyTool(readTool); err != nil {
		t.Fatalf("classify read tool: %v", err)
	}
	if readTool.info.Extra[metaReadOnly] != true {
		t.Fatalf("expected read-only metadata for git_status")
	}
	if readTool.info.Extra[metaDestructive] == true {
		t.Fatalf("did not expect destructive metadata for git_status")
	}

	writeTool := stubTool{info: &agentcore.ToolInfo{Name: "git_clean"}}
	if err := ClassifyTool(writeTool); err != nil {
		t.Fatalf("classify write tool: %v", err)
	}
	if writeTool.info.Extra[metaReadOnly] == true {
		t.Fatalf("did not expect read-only metadata for git_clean")
	}
	if writeTool.info.Extra[metaDestructive] != true {
		t.Fatalf("expected destructive metadata for git_clean")
	}
}

func TestClassifyToolSetsPolicyMetadata(t *testing.T) {
	fileTool := stubTool{info: &agentcore.ToolInfo{Name: "file_read"}}
	if err := ClassifyTool(fileTool); err != nil {
		t.Fatalf("classify file tool: %v", err)
	}
	if fileTool.info.Extra[metaReadOnly] != true {
		t.Fatal("expected read-only metadata")
	}
	if fileTool.info.Extra[metaApprovalStore] != approval.StoreFile {
		t.Fatalf("expected file approval metadata, got %v", fileTool.info.Extra[metaApprovalStore])
	}
	if fileTool.info.Extra[metaExternalPath] != true {
		t.Fatal("expected external path metadata")
	}

	scriptTool := stubTool{info: &agentcore.ToolInfo{Name: "bun_run_script"}}
	if err := ClassifyTool(scriptTool); err != nil {
		t.Fatalf("classify script tool: %v", err)
	}
	if scriptTool.info.Extra[metaDestructive] != true {
		t.Fatal("expected destructive metadata")
	}
	if scriptTool.info.Extra[metaSerialize] != true {
		t.Fatal("expected serialize metadata")
	}
}

func TestClassifyToolRequiresPolicyWhenMarked(t *testing.T) {
	tool := stubTool{info: &agentcore.ToolInfo{Name: "new_builtin_tool"}}
	if err := MarkPolicyRequired([]agentcore.Tool{tool}); err != nil {
		t.Fatalf("mark policy required: %v", err)
	}
	if err := ClassifyTool(tool); err == nil {
		t.Fatal("expected missing policy error")
	}
}

func TestClassifyToolAllowsUnmarkedExternalTool(t *testing.T) {
	tool := stubTool{info: &agentcore.ToolInfo{Name: "external_tool"}}
	if err := ClassifyTool(tool); err != nil {
		t.Fatalf("external tool should not require policy: %v", err)
	}
}

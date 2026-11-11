package toolpolicy

import (
	"context"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
)

type stubTool struct {
	info *runtimeport.ToolInfo
}

func (t stubTool) Info(context.Context) (*runtimeport.ToolInfo, error) {
	return t.info, nil
}

func (t stubTool) Invoke(context.Context, runtimeport.ToolInvocation) (*runtimeport.ToolResult, error) {
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

func TestSearchAskAndAttachmentToolNamesUsePolicy(t *testing.T) {
	for _, name := range []string{"search", "fetch", "ask_questions", "session_attachment_list", "session_attachment_read"} {
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
	readTool := stubTool{info: &runtimeport.ToolInfo{Name: "git_status"}}
	if err := ClassifyTool(readTool); err != nil {
		t.Fatalf("classify read tool: %v", err)
	}
	if !readTool.info.Policy.ReadOnly {
		t.Fatalf("expected read-only metadata for git_status")
	}
	if readTool.info.Policy.Destructive {
		t.Fatalf("did not expect destructive metadata for git_status")
	}

	writeTool := stubTool{info: &runtimeport.ToolInfo{Name: "git_clean"}}
	if err := ClassifyTool(writeTool); err != nil {
		t.Fatalf("classify write tool: %v", err)
	}
	if writeTool.info.Policy.ReadOnly {
		t.Fatalf("did not expect read-only metadata for git_clean")
	}
	if !writeTool.info.Policy.Destructive {
		t.Fatalf("expected destructive metadata for git_clean")
	}
}

func TestClassifyToolSetsPolicyMetadata(t *testing.T) {
	fileTool := stubTool{info: &runtimeport.ToolInfo{Name: "file_read"}}
	if err := ClassifyTool(fileTool); err != nil {
		t.Fatalf("classify file tool: %v", err)
	}
	if !fileTool.info.Policy.ReadOnly {
		t.Fatal("expected read-only metadata")
	}
	if fileTool.info.Policy.ApprovalStore != approval.StoreFile {
		t.Fatalf("expected file approval metadata, got %v", fileTool.info.Policy.ApprovalStore)
	}
	if !fileTool.info.Policy.ExternalPath {
		t.Fatal("expected external path metadata")
	}

	scriptTool := stubTool{info: &runtimeport.ToolInfo{Name: "bun_run_script"}}
	if err := ClassifyTool(scriptTool); err != nil {
		t.Fatalf("classify script tool: %v", err)
	}
	if !scriptTool.info.Policy.Destructive {
		t.Fatal("expected destructive metadata")
	}
	if !scriptTool.info.Policy.Serialize {
		t.Fatal("expected serialize metadata")
	}
}

func TestClassifyToolRequiresPolicyWhenMarked(t *testing.T) {
	tool := stubTool{info: &runtimeport.ToolInfo{Name: "new_builtin_tool"}}
	if err := MarkPolicyRequired([]runtimeport.Tool{tool}); err != nil {
		t.Fatalf("mark policy required: %v", err)
	}
	if err := ClassifyTool(tool); err == nil {
		t.Fatal("expected missing policy error")
	}
}

func TestClassifyToolAllowsUnmarkedExternalTool(t *testing.T) {
	tool := stubTool{info: &runtimeport.ToolInfo{Name: "external_tool"}}
	if err := ClassifyTool(tool); err != nil {
		t.Fatalf("external tool should not require policy: %v", err)
	}
}

func TestClassificationValidatesWholeListBeforeApplyingPolicies(t *testing.T) {
	first := stubTool{info: &runtimeport.ToolInfo{Name: "file_write", Policy: runtimeport.ToolPolicyMetadata{Required: true}}}
	missing := stubTool{info: &runtimeport.ToolInfo{Name: "missing", Policy: runtimeport.ToolPolicyMetadata{Required: true}}}
	before := first.info.Policy
	if err := ClassifyTools([]runtimeport.Tool{first, missing}); err == nil {
		t.Fatal("missing policy accepted")
	}
	if first.info.Policy != before {
		t.Fatal("failed classification partially changed policies")
	}
	if err := ClassifyTools([]runtimeport.Tool{first, first}); err == nil {
		t.Fatal("duplicate tool accepted")
	}
	if err := ClassifyTools([]runtimeport.Tool{first}); err != nil {
		t.Fatal(err)
	}
	if !first.info.Policy.Required || !first.info.Policy.Destructive || first.info.Policy.ApprovalStore != approval.StoreFile {
		t.Fatal(first.info.Policy)
	}
}

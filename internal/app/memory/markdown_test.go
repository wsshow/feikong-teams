package memory

import (
	"strings"
	"testing"
)

func TestBuildMemoryContextGroupsKnownTypes(t *testing.T) {
	context := BuildMemoryContext([]MemoryEntry{
		{Type: Preference, Summary: "偏好中文", Detail: "回答使用中文"},
		{Type: MemoryType("unknown"), Summary: "UNKNOWN_SENTINEL", Detail: "未知类型"},
	})

	if !strings.Contains(context, "## 长期记忆") || !strings.Contains(context, "### 用户偏好") {
		t.Fatalf("context = %q", context)
	}
	if !strings.Contains(context, "偏好中文") || strings.Contains(context, "UNKNOWN_SENTINEL") {
		t.Fatalf("context should include known entries only: %q", context)
	}
	if BuildMemoryContext(nil) != "" {
		t.Fatal("empty entries should produce empty context")
	}
	if BuildMemoryContext([]MemoryEntry{{Type: MemoryType("unknown"), Summary: "忽略"}}) != "" {
		t.Fatal("unknown-only entries should produce empty context")
	}
}

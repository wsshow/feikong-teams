package tools

import (
	"slices"
	"testing"
)

func TestBuiltinToolInfosExposeDescriptions(t *testing.T) {
	infos := BuiltinToolInfos()
	if len(infos) != len(BuiltinToolNames()) {
		t.Fatalf("tool info count = %d, want %d", len(infos), len(BuiltinToolNames()))
	}

	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Name)
		if info.DisplayName == "" {
			t.Fatalf("tool %s missing display name", info.Name)
		}
		if info.Description == "" {
			t.Fatalf("tool %s missing description", info.Name)
		}
		if info.Category == "" {
			t.Fatalf("tool %s missing category", info.Name)
		}
	}

	for _, name := range BuiltinToolNames() {
		if !slices.Contains(names, name) {
			t.Fatalf("tool catalog missing %s", name)
		}
	}
	if slices.Contains(names, "attachment") {
		t.Fatal("attachment should not be exposed in configurable tool catalog")
	}
}

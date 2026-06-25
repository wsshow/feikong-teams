package ssh

import (
	"context"
	"strings"
	"testing"
)

func TestGetTools(t *testing.T) {
	var nilTools *SSHTools
	if tools, err := nilTools.GetTools(); err == nil || tools != nil {
		t.Fatalf("nil GetTools tools=%#v err=%v, want error", tools, err)
	}
	if tools, err := (&SSHTools{}).GetTools(); err == nil || tools != nil {
		t.Fatalf("empty GetTools tools=%#v err=%v, want error", tools, err)
	}

	st := &SSHTools{client: NewClient("user", "pwd", "bad-address")}
	tools, err := st.GetTools()
	if err != nil {
		t.Fatalf("GetTools returned error: %v", err)
	}
	if len(tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(tools))
	}

	wantNames := []string{"ssh_execute", "ssh_upload", "ssh_download", "ssh_list_dir"}
	for i, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool %d info returned error: %v", i, err)
		}
		if info.Name != wantNames[i] {
			t.Fatalf("tool %d name = %q, want %q", i, info.Name, wantNames[i])
		}
		if strings.TrimSpace(info.Desc) == "" {
			t.Fatalf("tool %s description should not be empty", info.Name)
		}
	}
}

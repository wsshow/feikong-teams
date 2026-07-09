package agentsmd

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestIsMissingAgentsMD(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "os not exist", err: os.ErrNotExist, want: true},
		{name: "wrapped os not exist", err: fmt.Errorf("load agents: %w", os.ErrNotExist), want: true},
		{name: "file not found text", err: errors.New("file not found, skipping"), want: true},
		{name: "no such file text", err: errors.New("open AGENTS.md: no such file or directory"), want: true},
		{name: "permission denied", err: errors.New("permission denied"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingAgentsMD(tt.err); got != tt.want {
				t.Fatalf("isMissingAgentsMD() = %v, want %v", got, tt.want)
			}
		})
	}
}

//go:build !windows

package executil

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestCancellationKillsChildrenHoldingOutputPipes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5 & wait")
	start := time.Now()
	_, _, err := CombinedOutput(cmd, 1024)
	if err == nil {
		t.Fatal("cancelled process reported success")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("child kept pipe open after cancellation: %s", elapsed)
	}
}

package turn

import (
	"context"
	"testing"

	"fkteams/agentcore"
)

func TestChannelTargetHandlerOnlyTargetsSelectedInterrupt(t *testing.T) {
	ch := make(chan any, 1)
	ch <- "answer"

	targets, err := ChannelTargetHandler(ch, "ask-2")(context.Background(), []agentcore.Interrupt{
		{ID: "ask-1", IsRootCause: true},
		{ID: "ask-2", IsRootCause: true},
	})
	if err != nil {
		t.Fatalf("ChannelTargetHandler() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1: %#v", len(targets), targets)
	}
	if targets["ask-2"] != "answer" {
		t.Fatalf("ask-2 target = %#v, want answer", targets["ask-2"])
	}
	if _, ok := targets["ask-1"]; ok {
		t.Fatalf("ask-1 should not be targeted: %#v", targets)
	}
}

package agentcore

import "testing"

func TestMessageHelpers(t *testing.T) {
	if !(Message{}).IsEmpty() {
		t.Fatal("zero message should be empty")
	}
	if (Message{Role: RoleUser}).IsEmpty() {
		t.Fatal("message with role should not be empty")
	}

	message := Message{
		ContentParts: []ContentPart{
			{Type: ContentPartText, Text: "hello"},
			{Type: ContentPartImageURL, URL: "https://example.com/a.png"},
			{Type: ContentPartText, Text: "world"},
		},
	}
	if got := message.DisplayText(); got != "hello world" {
		t.Fatalf("DisplayText = %q", got)
	}
	message.Content = "direct"
	if got := message.DisplayText(); got != "direct" {
		t.Fatalf("DisplayText should prefer content, got %q", got)
	}
	message = Message{
		ContentParts: []ContentPart{{Type: ContentPartText, Text: "fallback"}},
	}
	if got := message.DisplayText(); got != "fallback" {
		t.Fatalf("DisplayText fallback = %q", got)
	}
}

func TestTurnInputAllMessages(t *testing.T) {
	input := TurnInput{
		Context: []Message{{Role: RoleSystem, Content: "system"}},
	}
	if got := input.AllMessages(); len(got) != 1 || got[0].Role != RoleSystem {
		t.Fatalf("AllMessages without message = %#v", got)
	}

	input.Message = Message{Role: RoleUser, Content: "hello"}
	got := input.AllMessages()
	if len(got) != 2 || got[1].Content != "hello" {
		t.Fatalf("AllMessages with message = %#v", got)
	}
}

func TestRunOptionsWithDefaults(t *testing.T) {
	opts := RunOptions{CheckpointID: "checkpoint-1"}.WithDefaults("default-run")

	if opts.RunID != "checkpoint-1" {
		t.Fatalf("run id = %q, want checkpoint-1", opts.RunID)
	}
	if opts.Sink == nil {
		t.Fatal("sink was not defaulted")
	}
	if err := opts.Sink(Event{}); err != nil {
		t.Fatalf("default sink returned error: %v", err)
	}
}

func TestRunOptionsWithDefaultsUsesFallbackRunID(t *testing.T) {
	opts := RunOptions{}.WithDefaults("default-run")

	if opts.RunID != "default-run" {
		t.Fatalf("run id = %q, want default-run", opts.RunID)
	}
}

package taskstream

import (
	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/domain/event"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

func TestPresentationSnapshotAndCursorAreAtomic(t *testing.T) {
	stream := NewManager().Register(StreamConfig{SessionID: "s", Cancel: func() {}, MaxRetainedEvents: 8})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 300; i++ {
			e := event.Event{Type: event.TypeAssistantText, EventID: fmt.Sprintf("event-%d", i), MessageID: "m", BlockID: "text", Content: "x", Sequence: int64(i + 1), SessionSequence: int64(i + 1)}
			stream.PublishRuntime(e, Event{"type": e.Type})
			runtime.Gosched()
		}
	}()
	for {
		state := stream.ConversationSnapshot()
		content := ""
		if len(state.View.Items) > 0 {
			content = state.View.Items[0].Node.Parts[0].Content
		}
		if len(content) != int(state.NextOffset) {
			t.Fatalf("snapshot contains %d chars at cursor %d", len(content), state.NextOffset)
		}
		select {
		case <-done:
			if state.NextOffset == 300 {
				return
			}
		default:
		}
		runtime.Gosched()
	}
}
func TestPublishedProjectionFramesAndSnapshotsCannotMutateEachOther(t *testing.T) {
	stream := NewManager().Register(StreamConfig{SessionID: "s", Cancel: func() {}})
	e := event.Event{Type: event.TypeAssistantText, EventID: "first", MessageID: "m", BlockID: "text", Content: "first", Sequence: 1}
	stream.PublishRuntime(e, Event{"type": e.Type})
	returned := stream.EventsSince(0)
	update := returned[0].Data["view_update"].(presentation.Update)
	update.Items[0].Node.Parts[0].Content = "mutated"
	e.EventID, e.Sequence, e.Content = "second", 2, " second"
	stream.PublishRuntime(e, Event{"type": e.Type})
	original := stream.EventsSince(0)[0].Data["view_update"].(presentation.Update)
	if original.Items[0].Node.Parts[0].Content != "first" {
		t.Fatal("subscriber mutated the replay window")
	}
	view, _ := stream.PresentationSnapshot()
	if view.Items[0].Node.Parts[0].Content != "first second" {
		t.Fatal("published frame mutated the live projection")
	}
}
func TestClientTerminalStateRequiresCommittedPresentation(t *testing.T) {
	stream := NewManager().Register(StreamConfig{SessionID: "s", Cancel: func() {}})
	stream.SetStatus("error")
	if stream.ClientStatus() != "processing" || stream.ConversationSnapshot().Status != "processing" {
		t.Fatal("client observed completion before authoritative view")
	}
	p := presentation.New("s")
	p.Apply(event.Event{Type: event.TypeError, EventID: "error", Sequence: 1, Error: "failed"})
	stream.PublishPresentation(p, Event{"type": event.TypeProcessingEnd, "status": "error"})
	state := stream.ConversationSnapshot()
	if state.Status != "error" || state.NextOffset != 1 || len(state.View.Items) != 1 || !strings.Contains(state.View.Items[0].Error.TechnicalDetail, "failed") {
		t.Fatalf("state=%#v", state)
	}
}

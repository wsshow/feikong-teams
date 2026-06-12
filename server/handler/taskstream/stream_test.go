package taskstream

import (
	"fkteams/agentcore"
	"testing"
	"time"
)

func newTestStream() *Stream {
	return NewManager().Register(StreamConfig{
		SessionID: "test-session",
		Cancel:    func() {},
	})
}

func TestSubmitInterruptRequiresPendingKind(t *testing.T) {
	s := newTestStream()

	if err := s.SubmitInterrupt(InterruptApproval, 1); err == nil {
		t.Fatal("expected submit without pending request to fail")
	}

	s.BeginInterrupt(InterruptApproval)
	if err := s.SubmitInterrupt(InterruptAsk, "answer"); err == nil {
		t.Fatal("expected submit with wrong interrupt kind to fail")
	}

	if err := s.SubmitInterrupt(InterruptApproval, 1); err != nil {
		t.Fatalf("expected approval submit to succeed: %v", err)
	}
	if err := s.SubmitInterrupt(InterruptApproval, 2); err == nil {
		t.Fatal("expected duplicate submit to fail")
	}

	got := <-s.InterruptCh()
	if got != 1 {
		t.Fatalf("expected first decision to be delivered, got %v", got)
	}

	s.CompleteInterrupt(InterruptApproval)
	if err := s.SubmitInterrupt(InterruptApproval, 1); err == nil {
		t.Fatal("expected submit after completion to fail")
	}
}

func TestBeginInterruptDrainsStaleDecision(t *testing.T) {
	s := newTestStream()
	s.interruptCh <- "stale"

	s.BeginInterrupt(InterruptAsk)
	if err := s.SubmitInterrupt(InterruptAsk, "fresh"); err != nil {
		t.Fatalf("expected ask submit to succeed: %v", err)
	}

	got := <-s.InterruptCh()
	if got != "fresh" {
		t.Fatalf("expected stale decision to be drained, got %v", got)
	}
}

func TestSubmitAskResponseRoutesByAskID(t *testing.T) {
	s := newTestStream()
	first, err := s.BeginAsk("ask-1")
	if err != nil {
		t.Fatalf("begin first ask: %v", err)
	}
	second, err := s.BeginAsk("ask-2")
	if err != nil {
		t.Fatalf("begin second ask: %v", err)
	}

	if err := s.SubmitAskResponse("ask-2", "second answer"); err != nil {
		t.Fatalf("submit second ask: %v", err)
	}
	select {
	case got := <-second:
		if got != "second answer" {
			t.Fatalf("second response = %v, want second answer", got)
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("timed out waiting for second ask response")
	}
	select {
	case got := <-first:
		t.Fatalf("first ask should still be pending, got %v", got)
	case <-time.After(20 * time.Millisecond):
	}

	if err := s.SubmitAskResponse("ask-1", "first answer"); err != nil {
		t.Fatalf("submit first ask: %v", err)
	}
	select {
	case got := <-first:
		if got != "first answer" {
			t.Fatalf("first response = %v, want first answer", got)
		}
	case <-time.After(20 * time.Millisecond):
		t.Fatal("timed out waiting for first ask response")
	}
}

func TestUnsubscribeDoesNotCancelTask(t *testing.T) {
	cancelled := make(chan struct{}, 1)
	s := NewManager().Register(StreamConfig{
		SessionID: "test-session",
		Cancel:    func() { cancelled <- struct{}{} },
	})

	ok, subID := s.Subscribe(FuncSubscriber(func(any) error { return nil }), 0)
	if !ok {
		t.Fatal("expected subscribe to succeed")
	}

	s.Unsubscribe(subID)

	select {
	case <-cancelled:
		t.Fatal("expected unsubscribe to detach without cancelling task")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestPublishFansOutToMultipleSubscribers(t *testing.T) {
	s := newTestStream()
	var first []Event
	var second []Event

	if ok, _ := s.Subscribe(FuncSubscriber(func(event any) error {
		first = append(first, event.(Event))
		return nil
	}), 0); !ok {
		t.Fatal("expected first subscribe to succeed")
	}
	if ok, _ := s.Subscribe(FuncSubscriber(func(event any) error {
		second = append(second, event.(Event))
		return nil
	}), 0); !ok {
		t.Fatal("expected second subscribe to succeed")
	}

	s.Publish(Event{"type": "message", "content": "hello"})

	if len(first) != 1 || first[0]["content"] != "hello" {
		t.Fatalf("expected first subscriber to receive event, got %#v", first)
	}
	if len(second) != 1 || second[0]["content"] != "hello" {
		t.Fatalf("expected second subscriber to receive event, got %#v", second)
	}
	if first[0]["stream_event_id"] != uint64(0) || second[0]["stream_event_id"] != uint64(0) {
		t.Fatalf("expected stream event id to be attached, got %#v %#v", first[0], second[0])
	}
}

func TestSubscribeReplaysEventsFromOffset(t *testing.T) {
	s := newTestStream()
	s.Publish(Event{"type": "message", "content": "first"})
	s.Publish(Event{"type": "message", "content": "second"})

	var replayed []Event
	if ok, _ := s.Subscribe(FuncSubscriber(func(event any) error {
		replayed = append(replayed, event.(Event))
		return nil
	}), 1); !ok {
		t.Fatal("expected subscribe to succeed")
	}

	if len(replayed) != 1 || replayed[0]["content"] != "second" {
		t.Fatalf("expected replay from offset 1, got %#v", replayed)
	}
}

func TestSteeringQueueIsConsumedBeforeFollowUpFallback(t *testing.T) {
	s := newTestStream()

	s.EnqueueMessage(QueuedMessage{Kind: QueueFollowUp, Text: "later"})
	s.EnqueueMessage(QueuedMessage{Kind: QueueSteering, Text: "change direction"})

	steering := s.TakeSteeringMessages(1)
	if len(steering) != 1 || steering[0].Text != "change direction" {
		t.Fatalf("expected one steering message, got %#v", steering)
	}
	if s.QueuedCount() != 1 {
		t.Fatalf("expected one queued follow-up, got %d", s.QueuedCount())
	}

	next, ok := s.DequeueNextMessage()
	if !ok || next.Kind != QueueFollowUp || next.Text != "later" {
		t.Fatalf("expected follow-up fallback, got %#v ok=%v", next, ok)
	}
}

func TestQueuedMessagesCanBeManagedBeforeConsumption(t *testing.T) {
	s := newTestStream()

	first := s.EnqueueMessage(QueuedMessage{Kind: QueueSteering, Text: "first"})
	second := s.EnqueueMessage(QueuedMessage{Kind: QueueSteering, Text: "second"})
	follow := s.EnqueueMessage(QueuedMessage{Kind: QueueFollowUp, Text: "later"})

	if first.ID == "" || second.ID == "" || follow.ID == "" {
		t.Fatal("queued messages should get stable IDs")
	}
	if updated, ok := s.UpdateQueuedMessage(second.ID, "changed", nil, "changed"); !ok || updated.Text != "changed" {
		t.Fatalf("expected second item to update, got %#v ok=%v", updated, ok)
	}
	if moved, ok := s.MoveQueuedMessage(second.ID, -1); !ok || moved.ID != second.ID {
		t.Fatalf("expected second item to move up, got %#v ok=%v", moved, ok)
	}
	if removed, ok := s.RemoveQueuedMessage(first.ID); !ok || removed.ID != first.ID {
		t.Fatalf("expected first item to be removed, got %#v ok=%v", removed, ok)
	}

	steering := s.TakeSteeringMessages(1)
	if len(steering) != 1 || steering[0].Text != "changed" {
		t.Fatalf("expected changed steering to remain first, got %#v", steering)
	}
	next, ok := s.DequeueNextMessage()
	if !ok || next.ID != follow.ID {
		t.Fatalf("expected follow-up after steering, got %#v ok=%v", next, ok)
	}
}

func TestQueuedMessageKindCanBeChangedBeforeConsumption(t *testing.T) {
	s := newTestStream()

	follow := s.EnqueueMessage(QueuedMessage{Kind: QueueFollowUp, Text: "later"})
	updated, ok := s.SetQueuedMessageKind(follow.ID, QueueSteering)
	if !ok || updated.Kind != QueueSteering {
		t.Fatalf("expected item to switch to steering, got %#v ok=%v", updated, ok)
	}
	if s.QueuedCount() != 1 {
		t.Fatalf("expected one queued item, got %d", s.QueuedCount())
	}
	steering := s.TakeSteeringMessages(1)
	if len(steering) != 1 || steering[0].ID != follow.ID {
		t.Fatalf("expected switched item to be consumed as steering, got %#v", steering)
	}
}

func TestQueuedMessageBuildsMultimodalUserMessage(t *testing.T) {
	msg := QueuedMessage{
		Text: "describe",
		Parts: []agentcore.ContentPart{
			{Type: agentcore.ContentPartText, Text: "describe"},
			{Type: agentcore.ContentPartImageURL, URL: "https://example.com/a.png"},
		},
	}.Message()

	if msg.Role != agentcore.RoleUser {
		t.Fatalf("expected user role, got %s", msg.Role)
	}
	if msg.Content != "" {
		t.Fatalf("expected text content to be cleared for multimodal input, got %q", msg.Content)
	}
	if len(msg.UserInputMultiContent) != 2 {
		t.Fatalf("expected multimodal parts to be preserved, got %#v", msg.UserInputMultiContent)
	}
}

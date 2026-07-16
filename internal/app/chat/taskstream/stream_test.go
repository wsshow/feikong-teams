package taskstream

import (
	"strings"
	"sync"
	"testing"
	"time"

	domainmessage "fkteams/internal/domain/message"
)

func newTestStream() *Stream {
	return NewManager().Register(StreamConfig{
		SessionID: "test-session",
		Cancel:    func() {},
	})
}

func TestEventBuilderKeepsTransportPayloadStable(t *testing.T) {
	parts := []domainmessage.ContentPart{{Type: domainmessage.ContentPartText, Text: "hello"}}
	event := Event{"type": "user_message", "session_id": "session-1", "content": "hello"}.
		With("queued", true).
		WithTurn("run-1", "turn-1").
		WithContentParts(parts)

	if event["type"] != "user_message" {
		t.Fatalf("type = %v, want user_message", event["type"])
	}
	if event["session_id"] != "session-1" || event["content"] != "hello" {
		t.Fatalf("unexpected event payload: %#v", event)
	}
	if event["run_id"] != "run-1" || event["turn_id"] != "turn-1" {
		t.Fatalf("missing turn metadata: %#v", event)
	}
	if got, ok := event["content_parts"].([]domainmessage.ContentPart); !ok || len(got) != 1 || got[0].Text != "hello" {
		t.Fatalf("content_parts = %#v", event["content_parts"])
	}
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

	ok, subID := s.Subscribe(FuncSubscriber(func(Event) error { return nil }), 0)
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
	first := make(chan Event, 1)
	second := make(chan Event, 1)

	if ok, _ := s.Subscribe(FuncSubscriber(func(event Event) error {
		first <- event
		return nil
	}), 0); !ok {
		t.Fatal("expected first subscribe to succeed")
	}
	if ok, _ := s.Subscribe(FuncSubscriber(func(event Event) error {
		second <- event
		return nil
	}), 0); !ok {
		t.Fatal("expected second subscribe to succeed")
	}

	s.Publish(Event{"type": "message", "content": "hello"})

	firstEvent := receiveEvent(t, first)
	secondEvent := receiveEvent(t, second)
	if firstEvent["content"] != "hello" || secondEvent["content"] != "hello" {
		t.Fatalf("expected both subscribers to receive event, got %#v %#v", firstEvent, secondEvent)
	}
	if firstEvent["stream_event_id"] != uint64(0) || secondEvent["stream_event_id"] != uint64(0) {
		t.Fatalf("expected stream event id to be attached, got %#v %#v", firstEvent, secondEvent)
	}
}

func TestSlowSubscriberDoesNotBlockPublishingOrState(t *testing.T) {
	s := newTestStream()
	started := make(chan struct{})
	release := make(chan struct{})
	if ok, _ := s.Subscribe(FuncSubscriber(func(Event) error {
		close(started)
		<-release
		return nil
	}), 0); !ok {
		t.Fatal("expected subscribe to succeed")
	}

	published := make(chan struct{})
	go func() {
		s.Publish(Event{"type": "message", "content": "hello"})
		close(published)
	}()

	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber blocked Publish")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not start")
	}

	s.SetStatus("cancelled")
	if got := s.Status(); got != "cancelled" {
		t.Fatalf("status = %q, want cancelled", got)
	}
	close(release)
}

func TestSubscriberReceivesEventsInStreamOrder(t *testing.T) {
	s := newTestStream()
	received := make(chan uint64, 100)
	if ok, _ := s.Subscribe(FuncSubscriber(func(event Event) error {
		received <- event["stream_event_id"].(uint64)
		return nil
	}), 0); !ok {
		t.Fatal("expected subscribe to succeed")
	}

	for i := 0; i < 100; i++ {
		s.Publish(Event{"type": "message"})
	}
	for want := uint64(0); want < 100; want++ {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("event ID = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for event %d", want)
		}
	}
}

func TestSlowSubscriberIsDetachedWhenItsQueueIsFull(t *testing.T) {
	s := newTestStream()
	started := make(chan struct{})
	release := make(chan struct{})
	if ok, _ := s.Subscribe(FuncSubscriber(func(Event) error {
		close(started)
		<-release
		return nil
	}), 0); !ok {
		t.Fatal("expected subscribe to succeed")
	}

	s.Publish(Event{"type": "message"})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not start")
	}
	for i := 0; i <= subscriberQueueSize; i++ {
		s.Publish(Event{"type": "message"})
	}
	if count := s.SubscriptionCount(); count != 0 {
		t.Fatalf("subscription count = %d, want 0 after queue overflow", count)
	}
	close(release)
}

func TestDoneDrainsQueuedEventsAndCancelsOnce(t *testing.T) {
	cancelled := 0
	s := NewManager().Register(StreamConfig{
		SessionID: "test-session",
		Cancel:    func() { cancelled++ },
	})
	received := make(chan uint64, 3)
	if ok, _ := s.Subscribe(FuncSubscriber(func(event Event) error {
		received <- event["stream_event_id"].(uint64)
		return nil
	}), 0); !ok {
		t.Fatal("expected subscribe to succeed")
	}

	for i := 0; i < 3; i++ {
		s.Publish(Event{"type": "message"})
	}
	s.Done()
	s.Done()
	s.Cancel()

	for want := uint64(0); want < 3; want++ {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("event ID = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for drained event %d", want)
		}
	}
	if count := s.SubscriptionCount(); count != 0 {
		t.Fatalf("subscription count after Done = %d, want 0", count)
	}
	if cancelled != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancelled)
	}
}

func TestPublishedAndReturnedEventsAreIsolated(t *testing.T) {
	s := newTestStream()
	original := Event{"type": "message", "content": "original"}
	s.Publish(original)
	original["content"] = "changed"

	first := s.EventsSince(0)
	if len(first) != 1 || first[0].Data["content"] != "original" {
		t.Fatalf("stored event was mutated through caller: %#v", first)
	}
	first[0].Data["content"] = "tampered"
	second := s.EventsSince(0)
	if second[0].Data["content"] != "original" {
		t.Fatalf("stored event was mutated through returned snapshot: %#v", second)
	}
	if _, exists := original["stream_event_id"]; exists {
		t.Fatal("Publish must not mutate the caller's event map")
	}
}

func TestEventPagesReturnAtomicPaginationMetadata(t *testing.T) {
	s := newTestStream()
	for i := 0; i < 5; i++ {
		s.Publish(Event{"type": "message", "index": i})
	}

	page := s.EventsPage(1, 2)
	if page.EventCount != 5 || page.SnapshotOffset != 1 || page.NextOffset != 3 || !page.MoreAvailable {
		t.Fatalf("page metadata = %#v", page)
	}
	if len(page.Events) != 2 || page.Events[0].ID != 1 || page.Events[1].ID != 2 {
		t.Fatalf("page events = %#v", page.Events)
	}

	tail := s.TailEventsPage(2)
	if tail.EventCount != 5 || tail.SnapshotOffset != 3 || tail.NextOffset != 5 || tail.MoreAvailable {
		t.Fatalf("tail metadata = %#v", tail)
	}
	if len(tail.Events) != 2 || tail.Events[0].ID != 3 || tail.Events[1].ID != 4 {
		t.Fatalf("tail events = %#v", tail.Events)
	}

	beyond := s.EventsPage(100, 2)
	if beyond.SnapshotOffset != 5 || beyond.NextOffset != 5 || len(beyond.Events) != 0 || beyond.MoreAvailable {
		t.Fatalf("beyond-tail page = %#v", beyond)
	}
}

func TestEventReplayWindowDropsOldestEvents(t *testing.T) {
	s := NewManager().Register(StreamConfig{
		SessionID:             "bounded-events",
		MaxRetainedEvents:     3,
		MaxRetainedEventBytes: 1 << 20,
	})
	for i := 0; i < 5; i++ {
		s.Publish(Event{"type": "message", "index": i})
	}

	page := s.EventsPage(0, 10)
	if !page.ReplayTruncated || page.EventCount != 5 || page.RetainedEventCount != 3 {
		t.Fatalf("page metadata = %#v", page)
	}
	if page.EarliestOffset != 2 || page.SnapshotOffset != 2 || page.NextOffset != 5 || page.MoreAvailable {
		t.Fatalf("page offsets = %#v", page)
	}
	if len(page.Events) != 3 || page.Events[0].ID != 2 || page.Events[2].ID != 4 {
		t.Fatalf("retained events = %#v", page.Events)
	}
	if s.CanReplay(1) || !s.CanReplay(2) {
		t.Fatal("CanReplay did not follow the retained window")
	}
	if ok, _, truncated := s.SubscribeChecked(FuncSubscriber(func(Event) error { return nil }), 1); ok || !truncated {
		t.Fatalf("SubscribeChecked() = (%v, %v), want truncated", ok, truncated)
	}
}

func TestOversizedEventIsNotRetained(t *testing.T) {
	s := NewManager().Register(StreamConfig{
		SessionID:             "oversized-event",
		MaxRetainedEvents:     10,
		MaxRetainedEventBytes: 32,
	})
	s.Publish(Event{"type": "message", "content": strings.Repeat("x", 64)})

	page := s.EventsPage(0, 10)
	if !page.ReplayTruncated || page.EventCount != 1 || page.RetainedEventCount != 0 || page.EarliestOffset != 1 {
		t.Fatalf("page = %#v", page)
	}
}

func TestReplayDoesNotHoldStreamLockDuringSubscriberWrite(t *testing.T) {
	s := newTestStream()
	s.Publish(Event{"type": "message", "content": "replay"})
	started := make(chan struct{})
	release := make(chan struct{})
	subscribed := make(chan bool, 1)
	var firstWrite sync.Once
	go func() {
		ok, _ := s.Subscribe(FuncSubscriber(func(Event) error {
			firstWrite.Do(func() {
				close(started)
				<-release
			})
			return nil
		}), 0)
		subscribed <- ok
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("replay did not start")
	}
	published := make(chan struct{})
	go func() {
		s.Publish(Event{"type": "message", "content": "live"})
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("replay write held the stream lock")
	}
	close(release)
	select {
	case ok := <-subscribed:
		if !ok {
			t.Fatal("expected subscribe to succeed")
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe did not finish")
	}
}

func TestSubscribeDrainsLiveTailWhenStreamFinishesDuringReplay(t *testing.T) {
	s := newTestStream()
	s.Publish(Event{"type": "message", "content": "replay"})
	started := make(chan struct{})
	release := make(chan struct{})
	received := make(chan string, 2)
	result := make(chan bool, 1)
	var firstWrite sync.Once
	go func() {
		ok, _ := s.Subscribe(FuncSubscriber(func(event Event) error {
			firstWrite.Do(func() {
				close(started)
				<-release
			})
			received <- event["content"].(string)
			return nil
		}), 0)
		result <- ok
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("replay did not start")
	}
	s.Publish(Event{"type": "message", "content": "tail"})
	s.Done()
	close(release)

	for _, want := range []string{"replay", "tail"} {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("content = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
	select {
	case ok := <-result:
		if ok {
			t.Fatal("completed stream must not retain a live subscription")
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe did not finish")
	}
}

func receiveEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber event")
		return nil
	}
}

func TestSubscribeReplaysEventsFromOffset(t *testing.T) {
	s := newTestStream()
	s.Publish(Event{"type": "message", "content": "first"})
	s.Publish(Event{"type": "message", "content": "second"})

	var replayed []Event
	if ok, _ := s.Subscribe(FuncSubscriber(func(event Event) error {
		replayed = append(replayed, event)
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

func TestRestoreQueuePreservesMessagesAcceptedDuringRestore(t *testing.T) {
	s := newTestStream()
	accepted := s.EnqueueMessage(QueuedMessage{
		ID:   "accepted",
		Kind: QueueFollowUp,
		Text: "new message",
	})

	s.RestoreQueue([]QueuedMessage{
		{ID: "persisted-steering", Kind: QueueSteering, Text: "old steering"},
		{ID: "persisted-follow-up", Kind: QueueFollowUp, Text: "old follow-up"},
		{ID: accepted.ID, Kind: QueueFollowUp, Text: "stale duplicate"},
	})

	queue := s.QueueSnapshot()
	if len(queue) != 3 {
		t.Fatalf("queue length = %d, want 3: %#v", len(queue), queue)
	}
	if queue[0].ID != "persisted-steering" || queue[1].ID != "persisted-follow-up" || queue[2].ID != accepted.ID {
		t.Fatalf("restored queue order = %#v", queue)
	}
	if queue[2].Text != "new message" {
		t.Fatalf("accepted message was replaced by stale snapshot: %#v", queue[2])
	}
}

func TestEnqueueAndCompletionTransitionAreAtomic(t *testing.T) {
	for i := 0; i < 1000; i++ {
		s := newTestStream()
		start := make(chan struct{})
		var accepted bool
		var dequeued bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, accepted = s.EnqueueMessageIfProcessing(QueuedMessage{Kind: QueueFollowUp, Text: "follow-up"})
		}()
		go func() {
			defer wg.Done()
			<-start
			_, dequeued = s.DequeueNextMessageOrComplete()
		}()
		close(start)
		wg.Wait()

		if accepted != dequeued {
			t.Fatalf("iteration %d accepted=%v dequeued=%v", i, accepted, dequeued)
		}
		if accepted {
			if s.Status() != "processing" {
				t.Fatalf("iteration %d status = %q after dequeue", i, s.Status())
			}
			if _, ok := s.DequeueNextMessageOrComplete(); ok || s.Status() != "completed" {
				t.Fatalf("iteration %d empty queue did not complete stream", i)
			}
		} else if s.Status() != "completed" {
			t.Fatalf("iteration %d rejected enqueue status = %q", i, s.Status())
		}
		if _, ok := s.EnqueueMessageIfProcessing(QueuedMessage{Text: "too late"}); ok {
			t.Fatalf("iteration %d accepted message after completion", i)
		}
	}
}

func TestCancellationAndCompletionTransitionAreAtomic(t *testing.T) {
	for i := 0; i < 1000; i++ {
		s := newTestStream()
		start := make(chan struct{})
		var cancelled bool
		var dequeued bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cancelled = s.CancelIfProcessing()
		}()
		go func() {
			defer wg.Done()
			<-start
			_, dequeued = s.DequeueNextMessageOrComplete()
		}()
		close(start)
		wg.Wait()

		if dequeued {
			t.Fatalf("iteration %d unexpectedly dequeued an empty queue", i)
		}
		if cancelled {
			if s.Status() != "cancelled" {
				t.Fatalf("iteration %d status = %q, want cancelled", i, s.Status())
			}
		} else if s.Status() != "completed" {
			t.Fatalf("iteration %d status = %q, want completed", i, s.Status())
		}
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
		Parts: []domainmessage.ContentPart{
			{Type: domainmessage.ContentPartText, Text: "describe"},
			{Type: domainmessage.ContentPartImageURL, URL: "https://example.com/a.png"},
		},
	}.Message()

	if msg.Role != domainmessage.RoleUser {
		t.Fatalf("expected user role, got %s", msg.Role)
	}
	if msg.Content != "" {
		t.Fatalf("expected text content to be cleared for multimodal input, got %q", msg.Content)
	}
	if len(msg.ContentParts) != 2 {
		t.Fatalf("expected multimodal parts to be preserved, got %#v", msg.ContentParts)
	}
}

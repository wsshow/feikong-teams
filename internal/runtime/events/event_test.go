package events

import (
	"context"
	"errors"
	"fkteams/internal/runtime/hooks"
	"testing"
	"time"
)

func TestDispatchEventDoesNotSerializeCallbacksGlobally(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)

	slowCtx := WithCallback(context.Background(), func(Event) error {
		close(started)
		<-release
		return nil
	})
	fastDone := make(chan error, 1)
	fastCtx := WithCallback(context.Background(), func(Event) error {
		return nil
	})

	go func() {
		firstDone <- DispatchEvent(slowCtx, Event{Type: EventMessageDelta, Content: "slow"})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow callback did not start")
	}

	go func() {
		fastDone <- DispatchEvent(fastCtx, Event{Type: EventMessageDelta, Content: "fast"})
	}()

	select {
	case err := <-fastDone:
		if err != nil {
			t.Fatalf("fast callback returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fast callback was blocked by unrelated slow callback")
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("slow callback returned error: %v", err)
	}
}

func TestNormalizeEventFillsCommonMetadata(t *testing.T) {
	event := NormalizeEvent(Event{Type: EventMessageDelta, RunID: "run_1", Content: "hello"})
	if event.EventID == "" {
		t.Fatal("event id was not set")
	}
	if event.Sequence == 0 {
		t.Fatal("sequence was not set")
	}
	if event.CreatedAt.IsZero() {
		t.Fatal("created_at was not set")
	}
	if event.RunID != "run_1" {
		t.Fatalf("run id = %q, want run_1", event.RunID)
	}
}

func TestIsMemberEventUsesMemberCallID(t *testing.T) {
	event := NormalizeEvent(Event{Type: EventMessageDelta, MemberCallID: "call_1"})
	if !IsMemberEvent(event) {
		t.Fatal("member event was not detected")
	}
}

func TestDispatchEventInvokesHooksBeforeCallback(t *testing.T) {
	bus := hooks.NewBus()
	bus.RegisterFunc("rewrite-event", []hooks.HookPoint{hooks.HookOnEvent}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		payload := inv.Payload.(hooks.EventPayload)
		payload.Event.Content = "hooked"
		return hooks.Result{Payload: payload}, nil
	}, hooks.Options{})
	ctx := hooks.WithBus(context.Background(), bus)

	var got Event
	ctx = WithCallback(ctx, func(event Event) error {
		got = event
		return nil
	})
	if err := DispatchEvent(ctx, Event{Type: EventMessageDelta, Content: "original"}); err != nil {
		t.Fatalf("dispatch event: %v", err)
	}
	if got.Content != "hooked" {
		t.Fatalf("content = %q, want hooked", got.Content)
	}
}

func TestDispatchEventHookCanSkipCallback(t *testing.T) {
	bus := hooks.NewBus()
	bus.RegisterFunc("skip-event", []hooks.HookPoint{hooks.HookOnEvent}, func(ctx hooks.Context, inv hooks.Invocation) (hooks.Result, error) {
		return hooks.Result{Action: hooks.ActionSkip}, nil
	}, hooks.Options{})
	ctx := hooks.WithBus(context.Background(), bus)

	called := false
	ctx = WithCallback(ctx, func(event Event) error {
		called = true
		return nil
	})
	if err := DispatchEvent(ctx, Event{Type: EventMessageDelta, Content: "hello"}); err != nil {
		t.Fatalf("dispatch event: %v", err)
	}
	if called {
		t.Fatal("callback should not be called")
	}
}

func TestDispatchAdapterAndNonInteractiveContext(t *testing.T) {
	ctx := WithNonInteractive(context.Background())
	if !IsNonInteractive(ctx) {
		t.Fatal("expected non-interactive context")
	}
	if IsNonInteractive(context.Background()) {
		t.Fatal("plain context should not be non-interactive")
	}

	var got Event
	ctx = WithCallback(ctx, func(event Event) error {
		got = event
		return nil
	})
	sink := Dispatch(ctx)
	if err := sink(Event{Type: EventMessageDelta, Content: "hello"}); err != nil {
		t.Fatalf("dispatch sink: %v", err)
	}
	if got.Content != "hello" || got.EventID == "" {
		t.Fatalf("dispatched event = %#v", got)
	}
}

func TestDispatchEventReturnsCallbackErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	ctx := WithCallback(context.Background(), func(event Event) error {
		return callbackErr
	})
	if err := DispatchEvent(ctx, Event{Type: EventMessageDelta}); !errors.Is(err, callbackErr) {
		t.Fatalf("callback error = %v, want %v", err, callbackErr)
	}
}

func TestInternalContinueContent(t *testing.T) {
	if !IsInternalContinueContent("Your previous text output was truncated, continue") {
		t.Fatal("expected text truncation message to be internal")
	}
	if !IsInternalContinueContent("Your previous tool call was truncated, continue") {
		t.Fatal("expected tool truncation message to be internal")
	}
	if IsInternalContinueContent("normal output") {
		t.Fatal("normal content should not be internal")
	}
}

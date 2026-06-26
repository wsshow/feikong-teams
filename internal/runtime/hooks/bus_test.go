package hooks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fkteams/internal/domain/message"
)

func TestBusInvokesHandlersInPriorityOrder(t *testing.T) {
	bus := NewBus()
	var got []string
	bus.RegisterFunc("second", []HookPoint{HookBeforeRun}, func(ctx Context, inv Invocation) (Result, error) {
		got = append(got, "second")
		return Result{}, nil
	}, Options{Priority: 20})
	bus.RegisterFunc("first", []HookPoint{HookBeforeRun}, func(ctx Context, inv Invocation) (Result, error) {
		got = append(got, "first")
		return Result{}, nil
	}, Options{Priority: 10})

	if _, err := bus.Invoke(context.Background(), Invocation{HookPoint: HookBeforeRun}); err != nil {
		t.Fatalf("invoke hooks: %v", err)
	}
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("order = %#v, want first, second", got)
	}
}

func TestBusPassesMutatedPayload(t *testing.T) {
	bus := NewBus()
	bus.RegisterFunc("mutate", []HookPoint{HookBeforeRun}, func(ctx Context, inv Invocation) (Result, error) {
		payload := inv.Payload.(BeforeRunPayload)
		payload.Input.Message.Content = "changed"
		return Result{Payload: payload}, nil
	}, Options{})

	result, err := bus.Invoke(context.Background(), Invocation{
		HookPoint: HookBeforeRun,
		Payload: BeforeRunPayload{Input: message.TurnInput{
			Message: message.Message{Role: message.RoleUser, Content: "original"},
		}},
	})
	if err != nil {
		t.Fatalf("invoke hooks: %v", err)
	}
	payload, ok := result.Payload.(BeforeRunPayload)
	if !ok || payload.Input.Message.Content != "changed" {
		t.Fatalf("payload = %#v, want changed before-run payload", result.Payload)
	}
}

func TestBusRejectsMismatchedPayloadPoint(t *testing.T) {
	bus := NewBus()
	_, err := bus.Invoke(context.Background(), Invocation{
		HookPoint: HookBeforeRun,
		Payload:   EventPayload{},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v, want payload mismatch", err)
	}
}

func TestBusWarnPolicyContinuesAfterError(t *testing.T) {
	bus := NewBus()
	called := false
	bus.RegisterFunc("bad", []HookPoint{HookOnEvent}, func(ctx Context, inv Invocation) (Result, error) {
		return Result{}, errors.New("boom")
	}, Options{ErrorPolicy: ErrorWarn})
	bus.RegisterFunc("next", []HookPoint{HookOnEvent}, func(ctx Context, inv Invocation) (Result, error) {
		called = true
		return Result{}, nil
	}, Options{})

	if _, err := bus.Invoke(context.Background(), Invocation{HookPoint: HookOnEvent}); err != nil {
		t.Fatalf("invoke hooks: %v", err)
	}
	if !called {
		t.Fatal("next hook was not called")
	}
}

func TestBusFailPolicyReturnsError(t *testing.T) {
	bus := NewBus()
	bus.RegisterFunc("bad", []HookPoint{HookBeforeRun}, func(ctx Context, inv Invocation) (Result, error) {
		return Result{}, errors.New("boom")
	}, Options{})

	if _, err := bus.Invoke(context.Background(), Invocation{HookPoint: HookBeforeRun}); err == nil {
		t.Fatal("expected error")
	}
}

func TestBusTimesOutHandler(t *testing.T) {
	bus := NewBus()
	bus.RegisterFunc("slow", []HookPoint{HookBeforeRun}, func(ctx Context, inv Invocation) (Result, error) {
		time.Sleep(time.Second)
		return Result{}, nil
	}, Options{Timeout: time.Millisecond})

	if _, err := bus.Invoke(context.Background(), Invocation{HookPoint: HookBeforeRun}); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestNilBusIsNoop(t *testing.T) {
	var bus *Bus
	input := message.TurnInput{Message: message.Message{Role: message.RoleUser, Content: "ping"}}

	got, err := bus.InvokeBeforeRun(context.Background(), input)
	if err != nil {
		t.Fatalf("nil bus before run: %v", err)
	}
	if got.Message.Content != "ping" {
		t.Fatalf("input = %#v, want unchanged", got)
	}
}

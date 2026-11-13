package chat

import (
	"context"
	"errors"
	"sync"
	"testing"

	"fkteams/internal/app/tools/ask"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

type interactionRunner func(context.Context, runtimeport.RunOptions) error

func (f interactionRunner) Run(ctx context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return &runtimeport.RunResult{}, f(ctx, opts)
}

func TestTurnRecordsEveryRootInteractionBeforePromptAndAnswer(t *testing.T) {
	h := &failingHistory{}
	roots := []runtimeport.Interrupt{
		{ID: "wrapper", Info: "wrapper"},
		{ID: "ask", IsRootCause: true, Info: &ask.AskInfo{Question: "choose", Options: []string{"A", "B"}, MultiSelect: true}},
		{ID: "approval", IsRootCause: true, Info: "run command?"},
	}
	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session", RunID: "run", History: h,
		Runner: interactionRunner(func(ctx context.Context, opts runtimeport.RunOptions) error {
			decisions, err := opts.InterruptHandler(ctx, roots)
			if err == nil && decisions["approval"] != 0 {
				t.Errorf("approval decision changed: %v", decisions)
			}
			return err
		}),
		InterruptHandler: func(_ context.Context, interrupts []runtimeport.Interrupt) (runtimeport.InterruptDecisions, error) {
			if len(interrupts) != 1 {
				t.Fatalf("interactions must receive independent decisions: %+v", interrupts)
			}
			if h.events[len(h.events)-1].Detail != interrupts[0].ID {
				t.Fatalf("request not recorded before prompt: %+v", h.events)
			}
			if interrupts[0].ID == "ask" {
				return runtimeport.InterruptDecisions{"ask": &ask.AskResponse{Selected: []string{"B"}, FreeText: "details"}}, nil
			}
			return runtimeport.InterruptDecisions{"approval": 0}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.events) != 4 {
		t.Fatalf("events = %+v", h.events)
	}
	for _, e := range h.events {
		if e.RunID != "run" || e.TurnID != "run:turn:1" || e.EventID == "" || e.CreatedAt.IsZero() {
			t.Fatalf("incomplete envelope: %+v", e)
		}
	}
	if got := h.events[1].Ask; got.ID != "ask" || len(got.Selected) != 1 || got.Selected[0] != "B" || got.FreeText != "details" {
		t.Fatalf("answer = %+v", got)
	}
	if got := h.events[3].Approval; got.ID != "approval" || got.Decision != "已拒绝" {
		t.Fatalf("approval = %+v", got)
	}
}

func TestTurnInteractionDefaultStillRejectsApproval(t *testing.T) {
	h := &failingHistory{}
	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session", History: h,
		Runner: interactionRunner(func(ctx context.Context, opts runtimeport.RunOptions) error {
			decisions, err := opts.InterruptHandler(ctx, []runtimeport.Interrupt{{ID: "approval", IsRootCause: true, Info: "run?"}})
			if decisions["approval"] != 0 {
				t.Fatalf("unsafe default: %+v", decisions)
			}
			return err
		}),
	})
	if err != nil || len(h.events) != 2 || h.events[1].Approval.Decision != "已拒绝" {
		t.Fatalf("run=%v events=%+v", err, h.events)
	}
}

func TestTurnInteractionFailuresNeverConfirmAnAnswer(t *testing.T) {
	for _, failSink := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "sink failure"}[failSink], func(t *testing.T) {
			expected := context.Canceled
			if failSink {
				expected = errors.New("sink unavailable")
			}
			called := false
			var received []event.Event
			_, err := NewService().RunTurn(context.Background(), TurnRequest{
				SessionID: "session",
				Runner: interactionRunner(func(ctx context.Context, opts runtimeport.RunOptions) error {
					_, err := opts.InterruptHandler(ctx, []runtimeport.Interrupt{{ID: "ask", IsRootCause: true, Info: &ask.AskInfo{Question: "choose"}}})
					return err
				}),
				EventSink: func(e event.Event) error {
					received = append(received, e)
					if failSink {
						return expected
					}
					return nil
				},
				InterruptHandler: func(context.Context, []runtimeport.Interrupt) (runtimeport.InterruptDecisions, error) {
					called = true
					return nil, expected
				},
			})
			if !errors.Is(err, expected) || called == failSink || len(received) != 1 || received[0].Type != event.TypeAskRequested {
				t.Fatalf("run=%v called=%v events=%+v", err, called, received)
			}
		})
	}
}

func TestConcurrentMemberQuestionsKeepTheirToolAndMemberIdentities(t *testing.T) {
	var mu sync.Mutex
	var received []event.Event
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session", RunID: "run",
		EventSink: func(e event.Event) error { mu.Lock(); received = append(received, e); mu.Unlock(); return nil },
		AskHandler: func(_ context.Context, req ask.RuntimeRequest) (*ask.AskResponse, error) {
			ready <- struct{}{}
			<-release
			return &ask.AskResponse{Selected: []string{req.Metadata.MemberCallID}}, nil
		},
		Runner: interactionRunner(func(ctx context.Context, _ runtimeport.RunOptions) error {
			results := make(chan error, 2)
			for _, id := range []string{"one", "two"} {
				go func(id string) {
					memberCtx := runtimeport.WithInterruptMetadata(ctx, runtimeport.InterruptMetadata{MemberCallID: id, MemberName: id})
					memberCtx = runtimeport.WithToolRuntimeMetadata(memberCtx, runtimeport.ToolRuntimeMetadata{CallID: "call-" + id, Name: "ask_questions"})
					_, err := ask.AskQuestions(memberCtx, &ask.AskRequest{Question: "choose " + id, Options: []string{"A", "B"}})
					results <- err
				}(id)
			}
			<-ready
			<-ready
			close(release)
			return errors.Join(<-results, <-results)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string][]event.Event)
	for _, e := range received {
		byID[e.Ask.ID] = append(byID[e.Ask.ID], e)
	}
	if len(byID) != 2 {
		t.Fatalf("questions=%+v", byID)
	}
	for _, pair := range byID {
		if len(pair) != 2 || pair[0].Type != event.TypeAskRequested || pair[1].Type != event.TypeAskAnswered || pair[0].MemberCallID != pair[1].Ask.Selected[0] || pair[1].ToolCallRef != "tool_call:call-"+pair[0].MemberCallID {
			t.Fatalf("member pair=%+v", pair)
		}
	}
}

func TestParallelApprovalDecisionsNeverAuthorizeAnotherOperation(t *testing.T) {
	roots := []runtimeport.Interrupt{{ID: "one", IsRootCause: true, Info: "first command"}, {ID: "two", IsRootCause: true, Info: "second command"}}
	calls := 0
	_, err := NewService().RunTurn(context.Background(), TurnRequest{
		SessionID: "session",
		Runner: interactionRunner(func(ctx context.Context, opts runtimeport.RunOptions) error {
			decisions, err := opts.InterruptHandler(ctx, roots)
			if err == nil && (decisions["one"] != 1 || decisions["two"] != 0) {
				t.Fatalf("decisions crossed operation boundary: %+v", decisions)
			}
			return err
		}),
		InterruptHandler: func(_ context.Context, interrupts []runtimeport.Interrupt) (runtimeport.InterruptDecisions, error) {
			calls++
			if len(interrupts) != 1 {
				t.Fatalf("multiple approval targets: %+v", interrupts)
			}
			decision := 0
			if interrupts[0].ID == "one" {
				decision = 1
			}
			return runtimeport.InterruptDecisions{interrupts[0].ID: decision}, nil
		},
	})
	if err != nil || calls != 2 {
		t.Fatalf("run=%v approval prompts=%d", err, calls)
	}
}

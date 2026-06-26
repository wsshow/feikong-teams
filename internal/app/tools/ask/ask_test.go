package ask

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtimeport "fkteams/internal/ports/runtime"
)

type askTestContextKey struct{}

type askRuntimeState struct {
	wasInterrupted bool
	resumeTarget   bool
	hasData        bool
	response       *AskResponse
}

type askRuntime struct {
	interruptErr error
	lastInfo     any
}

func (r *askRuntime) Interrupt(ctx context.Context, info any) error {
	r.lastInfo = info
	return r.interruptErr
}

func (r *askRuntime) GetInterruptState(ctx context.Context) (bool, bool, any) {
	state, _ := ctx.Value(askTestContextKey{}).(askRuntimeState)
	return state.wasInterrupted, false, nil
}

func (r *askRuntime) GetResumeContext(ctx context.Context, out any) (bool, bool) {
	state, _ := ctx.Value(askTestContextKey{}).(askRuntimeState)
	if state.response != nil {
		target := out.(**AskResponse)
		*target = state.response
	}
	return state.resumeTarget, state.hasData
}

func TestAskInfoString(t *testing.T) {
	info := &AskInfo{Question: "选哪个？"}
	if info.String() != "选哪个？" {
		t.Fatalf("String() = %q, want question", info.String())
	}
}

func TestAskQuestionsRequiresQuestion(t *testing.T) {
	result, err := AskQuestions(context.Background(), &AskRequest{})
	if err == nil || !strings.Contains(err.Error(), "question is required") {
		t.Fatalf("AskQuestions() = (%#v, %v), want question error", result, err)
	}
}

func TestAskQuestionsRequestsInterruptOnFirstCall(t *testing.T) {
	runtimeErr := errors.New("interrupt requested")
	runtime := &askRuntime{interruptErr: runtimeErr}
	ctx := runtimeport.WithInterruptRuntime(context.Background(), runtime)

	result, err := AskQuestions(ctx, &AskRequest{
		Question:    "继续吗？",
		Options:     []string{"继续", "停止"},
		MultiSelect: true,
	})
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("AskQuestions() error = %v, want interrupt error", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	info, ok := runtime.lastInfo.(*AskInfo)
	if !ok {
		t.Fatalf("interrupt info = %#v, want *AskInfo", runtime.lastInfo)
	}
	if info.Question != "继续吗？" || !info.MultiSelect || len(info.Options) != 2 {
		t.Fatalf("interrupt info = %#v", info)
	}
}

func TestAskQuestionsReturnsResumeResponse(t *testing.T) {
	ctx := runtimeport.WithInterruptRuntime(context.Background(), &askRuntime{})
	ctx = context.WithValue(ctx, askTestContextKey{}, askRuntimeState{
		wasInterrupted: true,
		resumeTarget:   true,
		hasData:        true,
		response:       &AskResponse{Selected: []string{"A"}, FreeText: "补充说明"},
	})

	result, err := AskQuestions(ctx, &AskRequest{Question: "选哪个？"})
	if err != nil {
		t.Fatalf("AskQuestions() error = %v", err)
	}
	if result.FreeText != "补充说明" || len(result.Selected) != 1 || result.Selected[0] != "A" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAskQuestionsUsesRuntimeHandlerForMemberAsk(t *testing.T) {
	called := false
	ctx := WithRuntimeHandler(context.Background(), func(_ context.Context, req RuntimeRequest) (*AskResponse, error) {
		called = true
		if req.ID == "" {
			t.Fatal("runtime request missing ID")
		}
		if req.Info.Question != "选哪个？" {
			t.Fatalf("question = %q, want 选哪个？", req.Info.Question)
		}
		if req.Metadata.MemberCallID != "member-call" {
			t.Fatalf("member call ID = %q, want member-call", req.Metadata.MemberCallID)
		}
		if req.ToolCallID != "ask-tool-call" || req.ToolName != "ask_questions" {
			t.Fatalf("tool identity = %q/%q, want ask-tool-call/ask_questions", req.ToolCallID, req.ToolName)
		}
		return &AskResponse{AskID: req.ID, Selected: []string{"A"}}, nil
	})
	ctx = runtimeport.WithToolRuntimeMetadata(ctx, runtimeport.ToolRuntimeMetadata{
		CallID: "ask-tool-call",
		Name:   "ask_questions",
	})
	ctx = runtimeport.WithInterruptMetadata(ctx, runtimeport.InterruptMetadata{
		MemberCallID:   "member-call",
		MemberToolName: "ask_fkagent_member",
		MemberName:     "member",
	})

	result, err := AskQuestions(ctx, &AskRequest{Question: "选哪个？"})
	if err != nil {
		t.Fatalf("AskQuestions() error = %v", err)
	}
	if !called {
		t.Fatal("runtime handler was not called")
	}
	if len(result.Selected) != 1 || result.Selected[0] != "A" {
		t.Fatalf("result = %#v, want selected A", result)
	}
}

func TestAskQuestionsReraisesInterruptForNonTargetResume(t *testing.T) {
	runtimeErr := errors.New("rerun interrupt")
	runtime := &askRuntime{interruptErr: runtimeErr}
	ctx := runtimeport.WithInterruptRuntime(context.Background(), runtime)
	ctx = context.WithValue(ctx, askTestContextKey{}, askRuntimeState{
		wasInterrupted: true,
		resumeTarget:   false,
	})

	result, err := AskQuestions(ctx, &AskRequest{Question: "选哪个？"})
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("AskQuestions() error = %v, want rerun interrupt", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if runtime.lastInfo != nil {
		t.Fatalf("reraised interrupt info = %#v, want nil", runtime.lastInfo)
	}
}

func TestAskQuestionsReportsMissingResumeData(t *testing.T) {
	ctx := runtimeport.WithInterruptRuntime(context.Background(), &askRuntime{})
	ctx = context.WithValue(ctx, askTestContextKey{}, askRuntimeState{
		wasInterrupted: true,
		resumeTarget:   true,
		hasData:        false,
	})

	result, err := AskQuestions(ctx, &AskRequest{Question: "选哪个？"})
	if err == nil || !strings.Contains(err.Error(), "no response received") {
		t.Fatalf("AskQuestions() = (%#v, %v), want missing response error", result, err)
	}
}

func TestGetTools(t *testing.T) {
	tools, err := GetTools()
	if err != nil {
		t.Fatalf("GetTools() error = %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	info, err := tools[0].Info(context.Background())
	if err != nil {
		t.Fatalf("tool info: %v", err)
	}
	if info.Name != "ask_questions" {
		t.Fatalf("tool name = %q, want ask_questions", info.Name)
	}
}

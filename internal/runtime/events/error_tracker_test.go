package events

import (
	"errors"
	"testing"
)

func TestErrorTrackerPreservesUnreportedAndMemberFailures(t *testing.T) {
	modelErr, saveErr := errors.New("model failed"), errors.New("save failed")
	var tracker ErrorTracker
	memberError := Error("member", "", modelErr)
	memberError.MemberCallID = "member-1"
	tracker.Record(memberError)
	if tracker.Unreported(modelErr) == nil {
		t.Fatal("member failure hid main failure")
	}
	tracker.Record(Error("main", "", modelErr))
	if got := tracker.Unreported(errors.Join(modelErr, nil)); got != nil {
		t.Fatalf("reported failure remains: %v", got)
	}
	got := tracker.Unreported(errors.Join(modelErr, saveErr))
	if !errors.Is(got, saveErr) || errors.Is(got, modelErr) {
		t.Fatalf("remaining failure = %v, want save failure only", got)
	}
}

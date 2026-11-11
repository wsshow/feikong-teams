package config

import (
	"errors"
	"testing"
)

func TestExecutionGateKeepsDependenciesUntilEveryRunFinishes(t *testing.T) {
	gate := &ExecutionGate{}
	first, second := gate.Begin(), gate.Begin()
	if _, err := gate.BeginUpdate(); !errors.Is(err, ErrExecutionsActive) {
		t.Fatalf("update during execution: %v", err)
	}
	first()
	first()
	if _, err := gate.BeginUpdate(); !errors.Is(err, ErrExecutionsActive) {
		t.Fatalf("update with second run active: %v", err)
	}
	second()
	update, err := gate.BeginUpdate()
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() { acquired <- gate.Begin() }()
	select {
	case release := <-acquired:
		release()
		t.Fatal("execution observed dependencies during update")
	default:
	}
	update()
	update()
	release := <-acquired
	release()
}

package handler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeShutdownWaitsForOwnedTasks(t *testing.T) {
	runtime := NewRuntime()
	started := make(chan struct{})
	release := make(chan struct{})
	if !runtime.Go(func() {
		close(started)
		<-release
	}) {
		t.Fatal("Go() rejected task before shutdown")
	}
	<-started

	done := make(chan error, 1)
	go func() { done <- runtime.Shutdown(context.Background()) }()
	select {
	case err := <-done:
		t.Fatalf("Shutdown() returned before task exit: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if runtime.Go(func() {}) {
		t.Fatal("Go() accepted task after shutdown started")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Shutdown(): %v", err)
	}
}

func TestRuntimeShutdownHonorsContext(t *testing.T) {
	runtime := NewRuntime()
	release := make(chan struct{})
	if !runtime.Go(func() { <-release }) {
		t.Fatal("Go() rejected task before shutdown")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := runtime.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want deadline exceeded", err)
	}
	close(release)
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown(): %v", err)
	}
}

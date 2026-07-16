package handler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	apptools "fkteams/internal/app/tools"
	runtimeport "fkteams/internal/ports/runtime"
	toolport "fkteams/internal/ports/tools"
)

type shutdownMCPProvider struct {
	clears atomic.Int32
}

func (*shutdownMCPProvider) GetToolsByName(context.Context, string) ([]runtimeport.Tool, error) {
	return nil, nil
}

func (*shutdownMCPProvider) GetAllToolGroups(context.Context) (toolport.MCPToolGroups, error) {
	return nil, nil
}

func (p *shutdownMCPProvider) ClearCache() {
	p.clears.Add(1)
}

func TestRuntimeShutdownWaitsForOwnedTasks(t *testing.T) {
	provider := &shutdownMCPProvider{}
	registry := apptools.NewToolGroupRegistry(apptools.ToolResolveContext{})
	registry.RegisterMCPProvider(provider)
	runtime := NewRuntime(RuntimeOptions{ToolRegistry: registry})
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
	if provider.clears.Load() != 0 {
		t.Fatal("MCP resources closed before owned tasks exited")
	}
	if runtime.Go(func() {}) {
		t.Fatal("Go() accepted task after shutdown started")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Shutdown(): %v", err)
	}
	if provider.clears.Load() != 1 {
		t.Fatalf("MCP cache clears = %d, want 1", provider.clears.Load())
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

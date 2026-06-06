package runner

import (
	"context"
	"fkteams/agentcore"
	"testing"
)

type cacheRunnerStub struct {
	_ byte
}

func (cacheRunnerStub) Run(context.Context, agentcore.TurnInput, agentcore.RunOptions) (*agentcore.RunResult, error) {
	return &agentcore.RunResult{}, nil
}

func TestCacheGetOrCreateReusesRunner(t *testing.T) {
	cache := NewCache()
	calls := 0
	factory := func() (agentcore.Runner, error) {
		calls++
		return &cacheRunnerStub{}, nil
	}

	first, err := cache.GetOrCreate(ModeTeam, factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := cache.GetOrCreate(ModeTeam, factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first != second {
		t.Fatal("expected cached runner to be reused")
	}
	if calls != 1 {
		t.Fatalf("expected factory to be called once, got %d", calls)
	}
}

func TestCacheClearRebuildsRunner(t *testing.T) {
	cache := NewCache()
	factory := func() (agentcore.Runner, error) {
		return &cacheRunnerStub{}, nil
	}

	first, err := cache.GetOrCreate(ModeTeam, factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cache.Clear()
	second, err := cache.GetOrCreate(ModeTeam, factory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first == second {
		t.Fatal("expected runner to be rebuilt after clear")
	}
}

func TestResolveFactoryDefaultsToTeam(t *testing.T) {
	key, factory, err := resolveFactory(context.Background(), "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != ModeTeam {
		t.Fatalf("expected team key, got %q", key)
	}
	if factory == nil {
		t.Fatal("expected factory")
	}
}

func TestResolveFactoryUnknownModeFallback(t *testing.T) {
	key, factory, err := resolveFactory(context.Background(), "unknown", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != ModeTeam {
		t.Fatalf("expected team fallback key, got %q", key)
	}
	if factory == nil {
		t.Fatal("expected factory")
	}
}

func TestResolveFactoryUnknownModeStrict(t *testing.T) {
	if _, _, err := resolveFactory(context.Background(), "unknown", "", false); err == nil {
		t.Fatal("expected unknown mode error")
	}
}

func TestResolveFactoryExplicitAgentNameUsesAgentCacheKey(t *testing.T) {
	key, factory, err := resolveFactory(context.Background(), "", "coder", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "agent_coder" {
		t.Fatalf("unexpected agent key: %q", key)
	}
	if factory == nil {
		t.Fatal("expected factory")
	}
}

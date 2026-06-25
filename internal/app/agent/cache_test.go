package agent

import (
	"context"
	"errors"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"sync"
	"testing"
)

type cacheRunnerStub struct {
	_ byte
}

func (cacheRunnerStub) Run(context.Context, domainmessage.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return &runtimeport.RunResult{}, nil
}

func TestCacheGetOrCreateReusesRunner(t *testing.T) {
	cache := NewCache()
	calls := 0
	factory := func() (runtimeport.Runner, error) {
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
	factory := func() (runtimeport.Runner, error) {
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

func TestCacheGetOrCreateDoesNotCacheErrors(t *testing.T) {
	cache := NewCache()
	calls := 0
	factory := func() (runtimeport.Runner, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("temporary")
		}
		return &cacheRunnerStub{}, nil
	}

	if _, err := cache.GetOrCreate(ModeTeam, factory); err == nil {
		t.Fatal("expected first factory error")
	}
	if _, err := cache.GetOrCreate(ModeTeam, factory); err != nil {
		t.Fatalf("second factory call should succeed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("factory calls = %d, want 2", calls)
	}
}

func TestCacheGetOrCreateRunsSingleFactoryConcurrently(t *testing.T) {
	cache := NewCache()
	var wg sync.WaitGroup
	start := make(chan struct{})
	calls := 0
	factory := func() (runtimeport.Runner, error) {
		calls++
		<-start
		return &cacheRunnerStub{}, nil
	}

	const workers = 5
	results := make(chan runtimeport.Runner, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := cache.GetOrCreate(ModeTeam, factory)
			if err != nil {
				t.Errorf("GetOrCreate error: %v", err)
				return
			}
			results <- r
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var first runtimeport.Runner
	for r := range results {
		if first == nil {
			first = r
			continue
		}
		if first != r {
			t.Fatal("expected all workers to receive the same runner")
		}
	}
	if calls != 1 {
		t.Fatalf("factory calls = %d, want 1", calls)
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

func TestResolveFactoryKnownModes(t *testing.T) {
	for _, mode := range []string{ModeRoundtable, ModeCustom, ModeDeep, ModeTeam, ModeSupervisor} {
		key, factory, err := resolveFactory(context.Background(), mode, "", false)
		if err != nil {
			t.Fatalf("resolveFactory(%q): %v", mode, err)
		}
		if key != mode {
			t.Fatalf("resolveFactory(%q) key = %q", mode, key)
		}
		if factory == nil {
			t.Fatalf("resolveFactory(%q) factory is nil", mode)
		}
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

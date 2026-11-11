package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fakeService struct {
	name     string
	order    *[]string
	startErr error
	stopErr  error
	stopFn   func(context.Context) error
}

func (s *fakeService) Name() string { return s.name }

func (s *fakeService) Start(context.Context) error {
	*s.order = append(*s.order, "start:"+s.name)
	return s.startErr
}

func (s *fakeService) Stop(ctx context.Context) error {
	*s.order = append(*s.order, "stop:"+s.name)
	if s.stopFn != nil {
		return s.stopFn(ctx)
	}
	return s.stopErr
}

func TestPhaseString(t *testing.T) {
	tests := map[Phase]string{
		PhaseInit:    "Init",
		PhaseSetup:   "Setup",
		PhaseStart:   "Start",
		PhaseReady:   "Ready",
		PhasePreStop: "PreStop",
		PhaseStop:    "Stop",
		PhaseCleanup: "Cleanup",
		Phase(99):    "Unknown(99)",
	}
	for phase, want := range tests {
		if got := phase.String(); got != want {
			t.Fatalf("Phase(%d).String() = %q, want %q", phase, got, want)
		}
	}
}

func TestApplicationConfigOptions(t *testing.T) {
	appDir := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", appDir)

	app := New(
		WithWorkspaceDir("/tmp/work"),
		WithMemoryEnabled(true),
		WithSchedulerEnabled(false),
		WithSchedulerDir("/tmp/scheduler"),
		WithInputHistoryPath("/tmp/input_history"),
		WithChatHistoryDir("/tmp/chat_history"),
		WithExitSignals(syscall.SIGTERM),
	)
	cfg := app.Config()
	if cfg.WorkspaceDir != "/tmp/work" ||
		!cfg.MemoryEnabled ||
		cfg.SchedulerEnabled ||
		cfg.SchedulerDir != "/tmp/scheduler" ||
		cfg.InputHistoryPath != "/tmp/input_history" ||
		cfg.ChatHistoryDir != "/tmp/chat_history" ||
		len(cfg.ExitSignals) != 1 ||
		cfg.ExitSignals[0] != syscall.SIGTERM {
		t.Fatalf("config = %#v", cfg)
	}

	defaultCfg := DefaultConfig()
	if defaultCfg.WorkspaceDir != filepath.Join(appDir, "workspace") {
		t.Fatalf("default workspace = %q", defaultCfg.WorkspaceDir)
	}
	if len(defaultCfg.ExitSignals) == 0 {
		t.Fatal("default exit signals should not be empty")
	}
}

func TestApplicationRunExecutesLifecycleInOrder(t *testing.T) {
	app := New(WithExitSignals())
	var order []string

	app.OnInit(func(context.Context) error {
		order = append(order, "hook:init")
		return nil
	})
	app.OnSetup(func(context.Context) error {
		order = append(order, "hook:setup")
		return nil
	})
	app.OnStart(func(context.Context) error {
		order = append(order, "hook:start")
		return nil
	})
	app.OnReady(func(context.Context) error {
		order = append(order, "hook:ready")
		app.Shutdown()
		return nil
	})
	app.OnPreStop(func(context.Context) error {
		order = append(order, "hook:prestop")
		return nil
	})
	app.OnStop(func(context.Context) error {
		order = append(order, "hook:stop")
		return nil
	})
	app.OnCleanup(func(context.Context) error {
		order = append(order, "hook:cleanup")
		return nil
	})
	app.RegisterService(&fakeService{name: "one", order: &order})
	app.RegisterService(&fakeService{name: "two", order: &order})

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := []string{
		"hook:init",
		"hook:setup",
		"start:one",
		"start:two",
		"hook:start",
		"hook:ready",
		"hook:prestop",
		"stop:two",
		"stop:one",
		"hook:stop",
		"hook:cleanup",
	}
	if got := strings.Join(order, ","); got != strings.Join(want, ",") {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
	if phase := app.CurrentPhase(); phase != PhaseCleanup {
		t.Fatalf("current phase = %s, want Cleanup", phase)
	}
}

func TestApplicationRunReturnsHookErrors(t *testing.T) {
	initErr := errors.New("bad init")
	app := New(WithExitSignals())
	app.OnInit(func(context.Context) error { return initErr })

	err := app.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "init failed") || !strings.Contains(err.Error(), "bad init") {
		t.Fatalf("Run init error = %v", err)
	}

	setupErr := errors.New("bad setup")
	app = New(WithExitSignals())
	app.OnSetup(func(context.Context) error { return setupErr })
	err = app.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "setup failed") || !strings.Contains(err.Error(), "bad setup") {
		t.Fatalf("Run setup error = %v", err)
	}
}

func TestApplicationRunStopsServicesOnStartError(t *testing.T) {
	app := New(WithExitSignals())
	var order []string
	app.RegisterService(&fakeService{name: "one", order: &order})
	app.RegisterService(&fakeService{name: "two", order: &order, startErr: errors.New("boom")})

	err := app.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start failed") || !strings.Contains(err.Error(), "two") {
		t.Fatalf("Run start error = %v", err)
	}

	want := []string{"start:one", "start:two", "stop:one"}
	if got := strings.Join(order, ","); got != strings.Join(want, ",") {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestApplicationRunStopsServicesOnStartHookError(t *testing.T) {
	app := New(WithExitSignals())
	var order []string
	app.RegisterService(&fakeService{name: "one", order: &order})
	app.OnStart(func(context.Context) error { return errors.New("bad hook") })

	err := app.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start hooks failed") {
		t.Fatalf("Run start hook error = %v", err)
	}

	want := []string{"start:one", "stop:one"}
	if got := strings.Join(order, ","); got != strings.Join(want, ",") {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestApplicationRunStopsOnContextCancel(t *testing.T) {
	app := New(WithExitSignals())
	ctx, cancel := context.WithCancel(context.Background())
	app.OnReady(func(context.Context) error {
		cancel()
		return nil
	})

	if err := app.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if phase := app.CurrentPhase(); phase != PhaseCleanup {
		t.Fatalf("current phase = %s, want Cleanup", phase)
	}
}

func TestApplicationShutdownContextBoundsCooperativeService(t *testing.T) {
	app := New(WithExitSignals(), WithShutdownTimeout(20*time.Millisecond))
	var order []string
	stopStarted := make(chan struct{})
	cleanupStarted := make(chan struct{})
	app.RegisterService(&fakeService{
		name:  "blocking",
		order: &order,
		stopFn: func(ctx context.Context) error {
			close(stopStarted)
			<-ctx.Done()
			return ctx.Err()
		},
	})
	app.OnReady(func(context.Context) error {
		app.Shutdown()
		return nil
	})
	app.OnCleanup(func(context.Context) error {
		close(cleanupStarted)
		return nil
	})

	startedAt := time.Now()
	if err := app.Run(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("shutdown elapsed = %v, want bounded shutdown", elapsed)
	}
	select {
	case <-stopStarted:
	default:
		t.Fatal("service stop was not attempted")
	}
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup hook was not attempted after timeout")
	}
}

func TestApplicationAccessorsAndShutdownBuffer(t *testing.T) {
	app := New(WithExitSignals())
	if app.ExitCh() == nil {
		t.Fatal("ExitCh should not be nil")
	}
	app.Shutdown()
	app.Shutdown()

	select {
	case sig := <-app.ExitCh():
		if sig != syscall.SIGTERM {
			t.Fatalf("shutdown signal = %v, want SIGTERM", sig)
		}
	default:
		t.Fatal("Shutdown should enqueue one signal")
	}

	select {
	case sig := <-app.ExitCh():
		t.Fatalf("Shutdown should not block or enqueue duplicate signal, got %v", sig)
	default:
	}
}

func TestApplicationCreatesState(t *testing.T) {
	app := New()
	if app.State() == nil {
		t.Fatal("application should create runtime state")
	}
}

func TestExecutePhaseCopiesHooks(t *testing.T) {
	app := New(WithExitSignals())
	var order []string
	app.OnInit(func(context.Context) error {
		order = append(order, "first")
		app.OnInit(func(context.Context) error {
			order = append(order, "late")
			return nil
		})
		return nil
	})
	app.OnInit(func(context.Context) error {
		order = append(order, "second")
		return nil
	})

	if err := app.executePhase(context.Background(), PhaseInit); err != nil {
		t.Fatalf("executePhase returned error: %v", err)
	}
	if got := strings.Join(order, ","); got != "first,second" {
		t.Fatalf("first execute order = %q", got)
	}
	if err := app.executePhase(context.Background(), PhaseInit); err != nil {
		t.Fatalf("second executePhase returned error: %v", err)
	}
	if got := strings.Join(order, ","); got != "first,second,first,second,late" {
		t.Fatalf("second execute order = %q", got)
	}
}

func TestStopServicesReturnsErrorsAndStillStopsEveryService(t *testing.T) {
	app := New(WithExitSignals())
	var order []string
	stopErr := errors.New("stop failed")
	app.RegisterService(&fakeService{name: "one", order: &order, stopErr: stopErr})
	app.RegisterService(&fakeService{name: "two", order: &order})

	if err := app.startServices(context.Background()); err != nil {
		t.Fatalf("startServices returned error: %v", err)
	}
	order = nil
	if err := app.stopServices(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("stop = %v", err)
	}
	if err := app.stopServices(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []string{"stop:two", "stop:one"}
	if got := strings.Join(order, ","); got != strings.Join(want, ",") {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestShutdownReturnsFailuresAndStillRunsAllCleanupHooks(t *testing.T) {
	app := New(WithExitSignals())
	stopErr, cleanupErr := errors.New("stop failed"), errors.New("cleanup failed")
	var order []string
	app.RegisterService(&fakeService{name: "one", order: &order, stopErr: stopErr})
	app.OnReady(func(context.Context) error { app.Shutdown(); return nil })
	app.OnCleanup(func(context.Context) error { order = append(order, "cleanup:first"); return cleanupErr })
	app.OnCleanup(func(context.Context) error { order = append(order, "cleanup:last"); return nil })
	err := app.Run(context.Background())
	if !errors.Is(err, stopErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("Run = %v", err)
	}
	if got := strings.Join(order, ","); got != "start:one,stop:one,cleanup:first,cleanup:last" {
		t.Fatalf("order = %s", got)
	}
}

func TestWaitForExitWithExitChannel(t *testing.T) {
	app := New(WithExitSignals())
	app.ExitCh() <- os.Interrupt
	app.waitForExit(context.Background())
}

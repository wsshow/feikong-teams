package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fkteams/internal/app/config"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"github.com/gin-gonic/gin"
)

type chatRunnerFunc func(context.Context, message.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error)

func (f chatRunnerFunc) Run(ctx context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	return f(ctx, input, opts)
}

func chatTestRouter(t *testing.T, runner chatRunnerFunc) (*Runtime, *gin.Engine) {
	t.Helper()
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	rt := newTestRuntime(t)
	t.Cleanup(rt.Close)
	if _, err := rt.RunnerCache.GetOrCreate("team", func() (runtimeport.Runner, error) { return runner, nil }); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/chat", rt.ChatHandlerWithState(nil))
	router.POST("/stream", rt.StreamStartHandlerWithState(nil))
	router.POST("/config", rt.UpdateConfigHandlerWithState(nil))
	return rt, router
}

func postChat(router http.Handler, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"session_id":"same","message":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func TestSyncChatReservesSessionAcrossTransports(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	_, router := chatTestRouter(t, func(ctx context.Context, _ message.TurnInput, _ runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		close(started)
		select {
		case <-release:
			return &runtimeport.RunResult{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- postChat(router, "/chat") }()
	<-started
	for _, path := range []string{"/chat", "/stream"} {
		response := postChat(router, path)
		if response.Code != http.StatusConflict {
			t.Errorf("%s: status=%d, body=%s", path, response.Code, response.Body.String())
		}
	}
	close(release)
	if response := <-finished; response.Code != http.StatusOK {
		t.Fatalf("first request: %d %s", response.Code, response.Body.String())
	}
}

func TestSyncChatReturnsPersistenceFailure(t *testing.T) {
	var rt *Runtime
	var router *gin.Engine
	rt, router = chatTestRouter(t, func(_ context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		dir := filepath.Join(rt.HistoryDir, "same")
		if err := os.Rename(dir, dir+".saved"); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dir, []byte("blocked"), 0600); err != nil {
			return nil, err
		}
		return &runtimeport.RunResult{}, opts.Sink(event.Event{Type: event.TypeAssistantCompleted, MessageID: "answer", Content: "done"})
	})
	response := postChat(router, "/chat")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, body=%s", response.Code, response.Body.String())
	}
	if status := rt.Streams.Get("same").Status(); status != "error" {
		t.Fatalf("status=%s", status)
	}
}

func TestShutdownCancelsAndWaitsForSyncChat(t *testing.T) {
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	rt, router := chatTestRouter(t, func(ctx context.Context, _ message.TurnInput, _ runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return nil, ctx.Err()
	})
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- postChat(router, "/chat") }()
	<-started
	stopped := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { stopped <- rt.Shutdown(ctx) }()
	select {
	case <-cancelled:
	case <-ctx.Done():
		close(release)
		t.Fatal("sync request was not cancelled")
	}
	select {
	case err := <-stopped:
		t.Errorf("shutdown returned before request exit: %v", err)
	default:
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	<-finished
}

func TestConfigUpdateRejectsActiveExecutionWithoutPublishing(t *testing.T) {
	rt, router := chatTestRouter(t, func(context.Context, message.TurnInput, runtimeport.RunOptions) (*runtimeport.RunResult, error) {
		return nil, nil
	})
	before := config.Get()
	release := rt.ExecutionGate.Begin()
	defer release()
	response := postChat(router, "/config")
	if response.Code != http.StatusConflict || config.Get() != before {
		t.Fatalf("status=%d, config was published=%v", response.Code, config.Get() != before)
	}
}

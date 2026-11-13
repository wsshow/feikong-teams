package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eventlog "fkteams/internal/adapters/storage/file/history"
	filequeue "fkteams/internal/adapters/storage/file/queue"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/chat/taskstream"
	"fkteams/internal/app/config"
	"fkteams/internal/app/execution"
	"fkteams/internal/domain/event"
	"fkteams/internal/domain/message"
	domainprocess "fkteams/internal/domain/process"
	runtimeport "fkteams/internal/ports/runtime"
	"github.com/gin-gonic/gin"
)

func TestStreamReportsAndPersistsRunErrorOnce(t *testing.T) {
	for _, reported := range []bool{false, true} {
		t.Run(fmt.Sprint(reported), func(t *testing.T) {
			runErr := errors.New("deepseek does not support image_url type")
			rt, router := chatTestRouter(t, func(_ context.Context, _ message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
				if reported {
					if err := opts.Sink(event.Event{Type: event.TypeError, Error: runErr.Error()}); err != nil {
						return nil, err
					}
				}
				return nil, runErr
			})
			response := postChat(router, "/stream")
			if response.Code != http.StatusOK {
				t.Fatalf("stream start: %d %s", response.Code, response.Body.String())
			}
			stream := rt.Streams.Get("same")
			deadline := time.Now().Add(3 * time.Second)
			for !stream.IsDone() && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if !stream.IsDone() || stream.Status() != "error" {
				t.Fatalf("stream did not finish with error: %s", stream.Status())
			}
			count := 0
			for _, entry := range stream.EventsSince(0) {
				if entry.Data["type"] == event.TypeError {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("live errors=%d, want one", count)
			}
			records, err := eventlog.LoadSessionTranscriptRecords(rt.sessionDirPath("same"), rt.Documents)
			if err != nil {
				t.Fatal(err)
			}
			count = 0
			for _, record := range records {
				if record.Event.Type == eventlog.TranscriptError {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("persisted errors=%d, want one", count)
			}
		})
	}
}

func TestHTTPConsumesCompletionForSyncAndStreaming(t *testing.T) {
	for _, path := range []string{"/chat", "/stream"} {
		t.Run(path, func(t *testing.T) {
			var rt *Runtime
			var inputs []message.TurnInput
			runner := chatRunnerFunc(func(_ context.Context, input message.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
				inputs = append(inputs, input)
				answer := "started"
				if len(inputs) == 1 {
					if err := rt.Queues.DeliverProcess(domainprocess.Record{ID: "process-http", Owner: domainprocess.Owner{SessionID: "same"}, State: domainprocess.Succeeded, Stdout: "HTTP result"}); err != nil {
						return nil, err
					}
				} else {
					answer = "completion received"
				}
				return &runtimeport.RunResult{}, opts.Sink(event.Event{Type: event.TypeAssistantCompleted, MessageID: opts.RunID + ":answer", Role: message.RoleAssistant, Content: answer, Message: &message.Message{Role: message.RoleAssistant, Content: answer}})
			})
			var router *gin.Engine
			rt, router = chatTestRouter(t, runner)
			response := postChat(router, path)
			if response.Code != http.StatusOK {
				t.Fatalf("request: %d %s", response.Code, response.Body.String())
			}
			stream := rt.Streams.Get("same")
			deadline := time.Now().Add(3 * time.Second)
			for !stream.IsDone() && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if !stream.IsDone() || stream.Status() != "completed" {
				t.Fatalf("did not complete: %s", stream.Status())
			}
			if len(inputs) != 2 || !strings.Contains(inputs[1].Message.Content, "HTTP result") {
				t.Fatalf("HTTP completion: %+v", inputs)
			}
		})
	}
}

func TestHTTPShutdownLeavesOtherApplicationSessionsRunning(t *testing.T) {
	queues := appchat.NewQueueService(taskstream.NewManager(), filequeue.NewStore(t.TempDir()), nil)
	externalCancelled := false
	external, _, err := queues.RegisterIfIdle(taskstream.StreamConfig{SessionID: "channel", Cancel: func() { externalCancelled = true }})
	if err != nil {
		t.Fatal(err)
	}
	defer external.Done()
	rt := NewRuntime(RuntimeOptions{HistoryDir: t.TempDir(), Dependencies: execution.Dependencies{Queues: queues}})
	if rt.Streams != queues.Streams() {
		t.Fatal("HTTP created a second session registry")
	}
	ctx, cancel := rt.newTaskContext(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	if !rt.Go(func() { <-ctx.Done(); close(stopped) }) {
		t.Fatal("HTTP task not admitted")
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("HTTP did not drain its own task")
	}
	if externalCancelled || queues.Streams().Get("channel") != external {
		t.Fatal("HTTP cancelled another entry's session")
	}
}

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

func TestQueueUpdateReturnsStorageFailureWithoutPublishing(t *testing.T) {
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(RuntimeOptions{HistoryDir: blocked})
	defer rt.Close()
	stream := rt.Streams.Register(taskstream.StreamConfig{SessionID: "session"})
	item, err := stream.EnqueueMessageIfProcessing(taskstream.QueuedMessage{Text: "before"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.PATCH("/:sessionID/:queueID", rt.StreamQueueUpdateHandler())
	req := httptest.NewRequest(http.MethodPatch, "/session/"+item.ID, strings.NewReader(`{"message":"after"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusInternalServerError || stream.QueueSnapshot()[0].Text != "before" {
		t.Fatalf("HTTP=%d queue=%v", response.Code, stream.QueueSnapshot())
	}
	if page := stream.EventsSince(0); len(page) != 0 {
		t.Fatalf("failed edit published events: %v", page)
	}
}

func TestRuntimeParentCancellationDrainsTasksAfterTimeout(t *testing.T) {
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	rt := newTestRuntime(t)
	parent, cancelParent := context.WithCancel(context.Background())
	if err := rt.Start(parent); err != nil {
		t.Fatal(err)
	}
	cancelled, release := make(chan struct{}), make(chan struct{})
	if !rt.Go(func() { <-rt.taskContext.Done(); close(cancelled); <-release }) {
		t.Fatal("task rejected")
	}
	cancelParent()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("lifecycle cancellation did not reach task")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := rt.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	select {
	case <-rt.shutdownDone:
		t.Fatal("runtime closed before task exited")
	default:
	}
	if rt.Go(func() {}) {
		t.Fatal("accepted task during shutdown")
	}
	close(release)
	select {
	case <-rt.shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("shutdown abandoned after timeout")
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

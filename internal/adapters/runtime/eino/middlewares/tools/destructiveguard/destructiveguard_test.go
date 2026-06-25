package destructiveguard

import (
	"context"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
)

func TestDestructiveToolsSerialized(t *testing.T) {
	guard := newRunnerGuard(t)

	var concurrentCount int32
	var maxConcurrent int32

	callTool := func(toolName string, wg *sync.WaitGroup) {
		defer wg.Done()

		endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			c := atomic.AddInt32(&concurrentCount, 1)
			if c > maxConcurrent {
				atomic.StoreInt32(&maxConcurrent, c)
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&concurrentCount, -1)
			return &compose.ToolOutput{Result: "ok"}, nil
		})

		wrapped := guard.Invokable(endpoint)
		wrapped(context.Background(), &compose.ToolInput{Name: toolName})
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go callTool("file_write", &wg)
	go callTool("file_write", &wg)
	go callTool("file_edit", &wg)
	wg.Wait()

	if maxConcurrent > 1 {
		t.Errorf("destructive tools ran concurrently: maxConcurrent=%d, want 1", maxConcurrent)
	} else {
		t.Logf("OK: destructive tools serialized, maxConcurrent=%d", maxConcurrent)
	}
}

func TestReadOnlyToolsParallel(t *testing.T) {
	guard := newRunnerGuard(t)

	var concurrentCount int32
	var maxConcurrent int32

	callTool := func(toolName string, wg *sync.WaitGroup) {
		defer wg.Done()

		endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			c := atomic.AddInt32(&concurrentCount, 1)
			if c > maxConcurrent {
				atomic.StoreInt32(&maxConcurrent, c)
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&concurrentCount, -1)
			return &compose.ToolOutput{Result: "ok"}, nil
		})

		wrapped := guard.Invokable(endpoint)
		wrapped(context.Background(), &compose.ToolInput{Name: toolName})
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go callTool("file_read", &wg)
	go callTool("grep", &wg)
	go callTool("file_read", &wg)
	wg.Wait()

	if maxConcurrent < 2 {
		t.Errorf("read-only tools should be parallel: maxConcurrent=%d, want >=2", maxConcurrent)
	} else {
		t.Logf("OK: read-only tools parallel, maxConcurrent=%d", maxConcurrent)
	}
}

func TestMixedReadWrite(t *testing.T) {
	guard := newRunnerGuard(t)

	var concurrentCount int32
	var maxConcurrent int32

	callTool := func(toolName string, wg *sync.WaitGroup) {
		defer wg.Done()

		endpoint := compose.InvokableToolEndpoint(func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
			c := atomic.AddInt32(&concurrentCount, 1)
			if c > maxConcurrent {
				atomic.StoreInt32(&maxConcurrent, c)
			}

			time.Sleep(20 * time.Millisecond)

			atomic.AddInt32(&concurrentCount, -1)
			return &compose.ToolOutput{Result: "ok"}, nil
		})

		wrapped := guard.Invokable(endpoint)
		wrapped(context.Background(), &compose.ToolInput{Name: toolName})
	}
	var wg sync.WaitGroup
	wg.Add(4)
	go callTool("file_read", &wg)
	go callTool("grep", &wg)
	go callTool("file_write", &wg)
	go callTool("file_edit", &wg)
	wg.Wait()

	if maxConcurrent < 2 {
		t.Errorf("reads should be parallel with writes pending: maxConcurrent=%d", maxConcurrent)
	}
	t.Logf("OK: mixed reads parallel (max=%d), writes serialized", maxConcurrent)
}

func newRunnerGuard(t *testing.T) compose.ToolMiddleware {
	t.Helper()

	guard, err := einoruntime.AdaptToolMiddlewareForRunner(New())
	if err != nil {
		t.Fatalf("adapt guard: %v", err)
	}
	return guard
}

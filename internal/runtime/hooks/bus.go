package hooks

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

const defaultTimeout = 3 * time.Second

type Options struct {
	Timeout     time.Duration
	ErrorPolicy ErrorPolicy
	Priority    int
}

type registeredHandler struct {
	handler Handler
	options Options
	seq     int64
}

type Bus struct {
	mu       sync.RWMutex
	nextSeq  int64
	handlers map[HookPoint][]registeredHandler
}

func NewBus() *Bus {
	return &Bus{handlers: make(map[HookPoint][]registeredHandler)}
}

func (b *Bus) Register(handler Handler, opts Options) func() {
	if b == nil || handler == nil {
		return func() {}
	}
	points := handler.Points()
	if len(points) == 0 {
		return func() {}
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}

	b.mu.Lock()
	b.nextSeq++
	seq := b.nextSeq
	for _, point := range points {
		if point == "" {
			continue
		}
		pointOpts := opts
		if pointOpts.ErrorPolicy == "" {
			pointOpts.ErrorPolicy = defaultErrorPolicy(point)
		}
		entry := registeredHandler{handler: handler, options: pointOpts, seq: seq}
		b.handlers[point] = append(b.handlers[point], entry)
		sortHandlers(b.handlers[point])
	}
	b.mu.Unlock()

	return func() {
		b.unregister(handler)
	}
}

func (b *Bus) RegisterFunc(name string, points []HookPoint, fn HandlerFunc, opts Options) func() {
	return b.Register(NewHandler(name, points, fn), opts)
}

func (b *Bus) unregister(handler Handler) {
	if b == nil || handler == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for point, handlers := range b.handlers {
		next := handlers[:0]
		for _, entry := range handlers {
			if entry.handler != handler {
				next = append(next, entry)
			}
		}
		if len(next) == 0 {
			delete(b.handlers, point)
			continue
		}
		b.handlers[point] = next
	}
}

func (b *Bus) Invoke(ctx context.Context, inv Invocation) (Result, error) {
	if inv.HookPoint == "" && inv.Payload != nil {
		inv.HookPoint = inv.Payload.HookPoint()
	}
	if b == nil || inv.HookPoint == "" {
		return Result{Payload: inv.Payload, Action: ActionContinue}, nil
	}
	if inv.Payload != nil && inv.Payload.HookPoint() != inv.HookPoint {
		return Result{Payload: inv.Payload, Action: ActionContinue}, fmt.Errorf("hook payload point %s does not match invocation point %s", inv.Payload.HookPoint(), inv.HookPoint)
	}
	handlers := b.handlersFor(inv.HookPoint)
	payload := inv.Payload
	result := Result{Payload: payload, Action: ActionContinue}
	for _, entry := range handlers {
		inv.Payload = payload
		next, err := invokeHandler(ctx, entry, inv)
		if err != nil {
			if shouldContinueAfterError(entry, inv.HookPoint, err) {
				continue
			}
			return result, err
		}
		if next.Payload != nil {
			payload = next.Payload
			result.Payload = payload
		}
		if next.Action != "" {
			result.Action = next.Action
		}
		if next.Message != "" {
			result.Message = next.Message
		}
		if result.Action == ActionSkip || result.Action == ActionReject {
			return result, nil
		}
	}
	return result, nil
}

func (b *Bus) handlersFor(point HookPoint) []registeredHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	handlers := b.handlers[point]
	result := make([]registeredHandler, len(handlers))
	copy(result, handlers)
	return result
}

func sortHandlers(handlers []registeredHandler) {
	sort.SliceStable(handlers, func(i, j int) bool {
		if handlers[i].options.Priority != handlers[j].options.Priority {
			return handlers[i].options.Priority < handlers[j].options.Priority
		}
		return handlers[i].seq < handlers[j].seq
	})
}

func invokeHandler(ctx context.Context, entry registeredHandler, inv Invocation) (Result, error) {
	timeout := entry.options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type response struct {
		result Result
		err    error
	}
	done := make(chan response, 1)
	go func() {
		var result Result
		var err error
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("hook %s panic: %v", entry.handler.Name(), recovered)
			}
			done <- response{result: result, err: err}
		}()
		result, err = entry.handler.Handle(hookCtx, inv)
		if err != nil {
			err = fmt.Errorf("hook %s failed: %w", entry.handler.Name(), err)
		}
	}()

	select {
	case resp := <-done:
		return resp.result, resp.err
	case <-hookCtx.Done():
		return Result{}, fmt.Errorf("hook %s timeout: %w", entry.handler.Name(), hookCtx.Err())
	}
}

func shouldContinueAfterError(entry registeredHandler, point HookPoint, err error) bool {
	policy := entry.options.ErrorPolicy
	if policy == "" {
		policy = defaultErrorPolicy(point)
	}
	switch policy {
	case ErrorIgnore:
		return true
	case ErrorWarn:
		log.Printf("hook warning: point=%s name=%s err=%v", point, entry.handler.Name(), err)
		return true
	default:
		return false
	}
}

func defaultErrorPolicy(point HookPoint) ErrorPolicy {
	switch point {
	case HookOnEvent, HookAfterRun, HookAfterToolCall, HookAfterModelResponse:
		return ErrorWarn
	default:
		return ErrorFail
	}
}

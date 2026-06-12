package agentcore

import (
	"context"
	"encoding/gob"
	"fmt"
)

type interruptMetadataContextKey struct{}

type InterruptRuntime interface {
	Interrupt(ctx context.Context, info any) error
	GetInterruptState(ctx context.Context) (bool, bool, any)
	GetResumeContext(ctx context.Context, out any) (bool, bool)
}

type InterruptMetadata struct {
	MemberCallID   string
	MemberToolName string
	MemberName     string
	MemberOrder    *int
}

type InterruptPayload struct {
	Info     any
	Metadata InterruptMetadata
}

var interruptRuntime InterruptRuntime

func init() {
	gob.Register(InterruptPayload{})
}

func RegisterInterruptRuntime(runtime InterruptRuntime) {
	interruptRuntime = runtime
}

func WithInterruptMetadata(ctx context.Context, metadata InterruptMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, interruptMetadataContextKey{}, metadata)
}

func InterruptMetadataFromContext(ctx context.Context) (InterruptMetadata, bool) {
	if ctx == nil {
		return InterruptMetadata{}, false
	}
	metadata, ok := ctx.Value(interruptMetadataContextKey{}).(InterruptMetadata)
	return metadata, ok
}

func RequestInterrupt(ctx context.Context, info any) error {
	if interruptRuntime == nil {
		return fmt.Errorf("interrupt runtime is not registered")
	}
	return interruptRuntime.Interrupt(ctx, info)
}

func GetInterruptState(ctx context.Context) (bool, bool, any) {
	if interruptRuntime == nil {
		return false, false, nil
	}
	return interruptRuntime.GetInterruptState(ctx)
}

func GetResumeContext[T any](ctx context.Context) (bool, bool, T) {
	var value T
	if interruptRuntime == nil {
		return false, false, value
	}
	isTarget, hasData := interruptRuntime.GetResumeContext(ctx, &value)
	return isTarget, hasData, value
}

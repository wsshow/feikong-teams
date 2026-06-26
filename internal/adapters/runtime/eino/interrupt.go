package eino

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"reflect"

	"github.com/cloudwego/eino/components/tool"
)

type interruptRuntime struct{}

// NewInterruptRuntime 创建 Eino HITL 中断适配器。
func NewInterruptRuntime() runtimeport.InterruptRuntime {
	return interruptRuntime{}
}

func (interruptRuntime) Interrupt(ctx context.Context, info any) error {
	if metadata, ok := runtimeport.InterruptMetadataFromContext(ctx); ok && metadata.MemberCallID != "" {
		return tool.Interrupt(ctx, runtimeport.InterruptPayload{
			Info:     info,
			Metadata: metadata,
		})
	}
	return tool.Interrupt(ctx, info)
}

func (interruptRuntime) GetInterruptState(ctx context.Context) (bool, bool, any) {
	return tool.GetInterruptState[any](ctx)
}

func (interruptRuntime) GetResumeContext(ctx context.Context, out any) (bool, bool) {
	isTarget, hasData, value := tool.GetResumeContext[any](ctx)
	if !hasData || out == nil {
		return isTarget, hasData
	}
	dst := reflect.ValueOf(out)
	if dst.Kind() != reflect.Pointer || dst.IsNil() || dst.Elem().Kind() == reflect.Invalid {
		return isTarget, hasData
	}
	src := reflect.ValueOf(value)
	if !src.IsValid() {
		return isTarget, hasData
	}
	if src.Type().AssignableTo(dst.Elem().Type()) {
		dst.Elem().Set(src)
	}
	return isTarget, hasData
}

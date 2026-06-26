package runtime

import (
	"context"
	"fmt"
)

type engineContextKey struct{}

// WithEngine 将 runtime engine 写入当前请求或应用生命周期上下文。
func WithEngine(ctx context.Context, engine Engine) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if engine == nil {
		return ctx
	}
	return context.WithValue(ctx, engineContextKey{}, engine)
}

// EngineFromContext 从上下文读取 runtime engine。
func EngineFromContext(ctx context.Context) (Engine, bool) {
	if ctx == nil {
		return nil, false
	}
	engine, ok := ctx.Value(engineContextKey{}).(Engine)
	return engine, ok && engine != nil
}

// RequireEngine 从上下文读取 runtime engine，缺失时返回明确错误。
func RequireEngine(ctx context.Context) (Engine, error) {
	if engine, ok := EngineFromContext(ctx); ok {
		return engine, nil
	}
	return nil, fmt.Errorf("runtime engine is not configured")
}

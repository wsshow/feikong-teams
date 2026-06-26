package hooks

import "context"

type Context = context.Context

type busKey struct{}

func WithBus(ctx context.Context, bus *Bus) context.Context {
	if bus == nil {
		return ctx
	}
	return context.WithValue(ctx, busKey{}, bus)
}

func FromContext(ctx context.Context) *Bus {
	if bus, ok := ctx.Value(busKey{}).(*Bus); ok && bus != nil {
		return bus
	}
	return nil
}

package claude

import (
	"github.com/cloudwego/eino/components/model"
)

type options struct {
	TopK *int32

	Thinking *Thinking

	DisableParallelToolUse *bool

	AutoCacheControl *CacheControl
}

func WithTopK(k int32) model.Option {
	return model.WrapImplSpecificOptFn(func(o *options) {
		o.TopK = &k
	})
}

func WithThinking(t *Thinking) model.Option {
	return model.WrapImplSpecificOptFn(func(o *options) {
		o.Thinking = t
	})
}

func WithDisableParallelToolUse() model.Option {
	return model.WrapImplSpecificOptFn(func(o *options) {
		b := true
		o.DisableParallelToolUse = &b
	})
}

// Deprecated: Use WithAutoCacheControl instead.
// WithEnableAutoCache enables automatic caching in a multi-turn conversation.
// The caching strategy sets separate breakpoints for tool and system messages.
// Additionally, a breakpoint is set on the last input message of each turn to cache the session.
func WithEnableAutoCache(enabled bool) model.Option {
	return model.WrapImplSpecificOptFn(func(o *options) {
		if enabled {
			o.AutoCacheControl = &CacheControl{}
		} else {
			o.AutoCacheControl = nil
		}
	})
}

// WithAutoCacheControl sets the CacheControl for automatically placed cache breakpoints.
// The caching strategy sets separate breakpoints for tool and system messages.
// Additionally, a breakpoint is set on the last input message of each turn to cache the session.
// A non-nil ctrl enables auto cache; nil disables it.
func WithAutoCacheControl(ctrl *CacheControl) model.Option {
	return model.WrapImplSpecificOptFn(func(o *options) {
		o.AutoCacheControl = ctrl
	})
}

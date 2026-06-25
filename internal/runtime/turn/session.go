package turn

import (
	"context"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/hooks"
)

type EventHandler func(events.Event) error
type StartHandler func(context.Context)
type FinishHandler func(context.Context, *runtimeport.RunResult, error)

// Session 提供面向一次会话执行的易用接口。
type Session struct {
	engine *core
	cfg    runConfig
}

func NewSession(runner runtimeport.Runner, checkpointID string) *Session {
	return &Session{engine: newEngine(runner, checkpointID)}
}

func (s *Session) WithInput(input TurnInput) *Session {
	s.cfg.Input = input
	return s
}

func (s *Session) WithMessage(message message.Message) *Session {
	s.cfg.Input.Message = message
	return s
}

func (s *Session) WithText(text string) *Session {
	return s.WithMessage(message.Message{
		Role:    message.RoleUser,
		Content: text,
	})
}

func (s *Session) WithMessages(messages []message.Message) *Session {
	s.cfg.Input.Context = messages
	return s
}

func (s *Session) WithRunID(runID string) *Session {
	s.cfg.RunID = runID
	return s
}

func (s *Session) OnEvent(handler EventHandler) *Session {
	s.cfg.EventCallback = handler
	return s
}

func (s *Session) WithHistory(history HistorySink) *Session {
	s.cfg.Recorder = history
	return s
}

func (s *Session) OnStart(handler StartHandler) *Session {
	s.cfg.OnStart = handler
	return s
}

func (s *Session) OnInterrupt(handler InterruptHandler) *Session {
	s.cfg.OnInterrupt = handler
	return s
}

func (s *Session) NonInteractive() *Session {
	s.cfg.NonInteractive = true
	return s
}

func (s *Session) WithContext(hook ContextHook) *Session {
	if hook != nil {
		s.cfg.ContextHooks = append(s.cfg.ContextHooks, hook)
	}
	return s
}

func (s *Session) WithHookBus(bus *hooks.Bus) *Session {
	s.cfg.HookBus = bus
	return s
}

func (s *Session) OnFinish(handler FinishHandler) *Session {
	s.cfg.OnFinish = handler
	return s
}

func (s *Session) Run(ctx context.Context) (*runtimeport.RunResult, error) {
	return s.engine.run(ctx, s.cfg)
}

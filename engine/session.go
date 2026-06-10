package engine

import (
	"context"
	"fkteams/agentcore"
	"fkteams/events"
	"fkteams/hooks"
)

type EventHandler func(events.Event) error
type StartHandler func(context.Context)
type FinishHandler func(context.Context, *agentcore.RunResult, error)

// Session 提供面向一次会话执行的易用接口。
type Session struct {
	engine *core
	cfg    runConfig
}

func NewSession(runner agentcore.Runner, checkpointID string) *Session {
	return &Session{engine: newEngine(runner, checkpointID)}
}

func (s *Session) WithInput(input TurnInput) *Session {
	s.cfg.Input = input
	return s
}

func (s *Session) WithMessage(message agentcore.Message) *Session {
	s.cfg.Input.Message = message
	return s
}

func (s *Session) WithText(text string) *Session {
	return s.WithMessage(agentcore.Message{
		Role:    agentcore.RoleUser,
		Content: text,
	})
}

func (s *Session) WithMessages(messages []agentcore.Message) *Session {
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

func (s *Session) Run(ctx context.Context) (*agentcore.RunResult, error) {
	return s.engine.run(ctx, s.cfg)
}

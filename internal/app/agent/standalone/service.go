// Package standalone 提供轻量独立智能体执行门面。
package standalone

import (
	"context"
	"fmt"
	"strings"

	agentscommon "fkteams/internal/app/agent/catalog/common"
	"fkteams/internal/app/chat"
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	storageport "fkteams/internal/ports/storage"
)

// TextDeltaSink 接收主智能体的文本增量。
type TextDeltaSink func(delta string) error

// Dependencies 是独立智能体运行需要的最小 runtime 能力。
type Dependencies struct {
	AgentRuntime  runtimeport.AgentRuntime
	RunnerRuntime runtimeport.RunnerRuntime
}

// Request 描述一次轻量独立智能体调用。
type Request struct {
	Name         string
	Description  string
	Instruction  string
	TemplateVars map[string]any

	Model runtimeport.ChatModel
	Tools []runtimeport.Tool

	Context []domainmessage.Message
	Message domainmessage.Message
	Input   string

	RunID           string
	CheckpointStore storageport.CheckpointStore
	EventSink       runtimeport.EventSink
}

// Result 描述一次独立智能体运行结果。
type Result struct {
	Text      string
	LastEvent domainevent.Event
}

// Service 封装 Bare profile agent 的创建、runner 执行和文本收集。
type Service struct {
	deps Dependencies
}

// NewService 创建独立智能体服务。
func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

// RunText 执行非流式文本任务并返回最终文本。
func (s *Service) RunText(ctx context.Context, req Request) (string, error) {
	result, err := s.run(ctx, req, false, nil)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// StreamText 执行流式文本任务。onDelta 接收文本增量，返回值仍是完整文本。
func (s *Service) StreamText(ctx context.Context, req Request, onDelta TextDeltaSink) (string, error) {
	result, err := s.run(ctx, req, true, onDelta)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// Run 执行轻量独立智能体并返回结构化结果，适合需要自定义 EventSink 的调用方。
func (s *Service) Run(ctx context.Context, req Request) (*Result, error) {
	return s.run(ctx, req, false, nil)
}

func (s *Service) run(ctx context.Context, req Request, streaming bool, onDelta TextDeltaSink) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.validate(req); err != nil {
		return nil, err
	}

	ctx = runtimeport.WithAgentRuntime(ctx, s.deps.AgentRuntime)
	agent, err := agentscommon.BuildAgent(ctx, agentscommon.Definition{
		Name:         req.Name,
		Description:  req.Description,
		Instruction:  req.Instruction,
		TemplateVars: req.TemplateVars,
		Profile:      agentscommon.ProfileBare,
		Model:        req.Model,
		Tools:        req.Tools,
	})
	if err != nil {
		return nil, fmt.Errorf("build standalone agent: %w", err)
	}

	runner, err := s.deps.RunnerRuntime.NewRunner(ctx, runtimeport.RunnerConfig{
		Agent:           agent,
		EnableStreaming: streaming,
		CheckpointStore: req.CheckpointStore,
	})
	if err != nil {
		return nil, fmt.Errorf("create standalone runner: %w", err)
	}

	var text chat.AssistantTextCollector
	result, err := chat.NewService().RunTurn(ctx, chat.TurnRequest{
		SessionID:      req.Name,
		RunID:          req.RunID,
		Runner:         runner,
		NonInteractive: true,
		Input:          domainmessage.TurnInput{Context: req.Context, Message: requestMessage(req)},
		EventSink: func(event domainevent.Event) error {
			if delta := text.Record(event); delta != "" && onDelta != nil {
				if err := onDelta(delta); err != nil {
					return err
				}
			}
			if req.EventSink != nil {
				return req.EventSink(event)
			}
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("run standalone agent: %w", err)
	}

	final := Result{Text: strings.TrimSpace(text.Text())}
	if result != nil {
		final.LastEvent = result.LastEvent
	}
	return &final, nil
}

func (s *Service) validate(req Request) error {
	if s == nil {
		return fmt.Errorf("standalone service is nil")
	}
	if s.deps.AgentRuntime == nil {
		return fmt.Errorf("agent runtime is required")
	}
	if s.deps.RunnerRuntime == nil {
		return fmt.Errorf("runner runtime is required")
	}
	if req.Name == "" {
		return fmt.Errorf("standalone agent name is required")
	}
	if req.Model == nil {
		return fmt.Errorf("standalone agent model is required")
	}
	if requestMessage(req).IsEmpty() {
		return fmt.Errorf("standalone agent input is required")
	}
	return nil
}

func requestMessage(req Request) domainmessage.Message {
	if !req.Message.IsEmpty() {
		return req.Message
	}
	if strings.TrimSpace(req.Input) == "" {
		return domainmessage.Message{}
	}
	return domainmessage.Message{Role: domainmessage.RoleUser, Content: req.Input}
}

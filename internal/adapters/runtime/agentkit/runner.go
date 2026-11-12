package agentkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	storageport "fkteams/internal/ports/storage"
	"fkteams/internal/runtime/events"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	kit "github.com/wsshow/agentkit"
)

// Runner 用 AgentKit 执行声明，并输出项目的稳定事件协议。
type Runner struct {
	agent     *agent
	streaming bool
	store     storageport.CheckpointStore
}

// Run 为本次调用创建隔离会话，在同一会话内处理所有审批恢复。
func (r *Runner) Run(ctx context.Context, input domainmessage.TurnInput, opts runtimeport.RunOptions) (*runtimeport.RunResult, error) {
	if r == nil || r.agent == nil {
		return nil, fmt.Errorf("runner is nil")
	}
	opts = opts.WithDefaults(uuid.NewString())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	turnID := events.TurnID(opts.RunID, 1)
	c := newConverter(events.NewEmitter(opts.RunID, turnID, opts.Sink), cancel)
	for _, event := range []domainevent.Event{events.AgentStart(opts.RunID), events.TurnStart(opts.RunID, turnID)} {
		if err := c.emit(event); err != nil {
			return nil, err
		}
	}
	if !input.Message.IsEmpty() && input.Message.Role == domainmessage.RoleUser {
		if err := c.emit(events.UserMessage(opts.RunID, turnID, turnID+":user", input.Message)); err != nil {
			return nil, err
		}
	}
	history := AdaptMessages(input.AllMessages())
	var err error
	if len(r.agent.loop) == 0 {
		_, err = r.execute(ctx, r.agent, history, opts, c)
	} else {
		for round := 0; (r.agent.iterations == 0 || round < r.agent.iterations) && err == nil; round++ {
			for _, member := range r.agent.loop {
				history, err = r.execute(ctx, member, history, opts, c)
				if err != nil {
					break
				}
			}
		}
	}
	if sinkErr := c.err(); sinkErr != nil {
		err = sinkErr
	}
	// 生命周期始终闭合，执行错误同时作为返回值交给用例层。
	if err != nil && !errors.Is(err, context.Canceled) {
		_ = c.emit(events.Error(r.agent.Name(), "", err))
	}
	turnEnd := events.TurnEnd(opts.RunID, turnID)
	if errors.Is(err, context.Canceled) {
		turnEnd.Type = domainevent.TypeTurnCancelled
	} else if err != nil {
		turnEnd.Type = domainevent.TypeTurnFailed
		turnEnd.Error = err.Error()
	}
	endErr := c.emit(turnEnd)
	if err == nil {
		err = endErr
	}
	end := events.AgentEnd(opts.RunID)
	if err != nil {
		end.Error = err.Error()
	}
	if endErr := c.emit(end); err == nil {
		err = endErr
	}
	return &runtimeport.RunResult{LastEvent: c.lastEvent()}, err
}

func (r *Runner) execute(ctx context.Context, definition *agent, history []*schema.Message, opts runtimeport.RunOptions, c *converter) (output []*schema.Message, runErr error) {
	if err := ctx.Err(); err != nil {
		return history, err
	}
	cfg := definition.config
	cfg.History = history
	cfg.CheckPointStore = r.store
	observation := c.observeModel(cfg.Model, cfg.Name, r.streaming)
	cfg.Model = observation
	cfg.Handlers = append(append([]kit.ChatModelAgentMiddleware(nil), cfg.Handlers...), observation)
	cfg.SubAgents = append([]kit.SubAgentConfig(nil), cfg.SubAgents...)
	c.registerMembers(definition.members)
	for i := range cfg.SubAgents {
		child := &cfg.SubAgents[i]
		observation := c.observeModel(child.Model, child.Name, r.streaming)
		child.Model = observation
		child.Handlers = append(append([]kit.ChatModelAgentMiddleware(nil), child.Handlers...), observation)
		child.ToolPolicy = c.toolPolicy(child.ToolPolicy, child.Name)
	}
	cfg.ToolPolicy = c.toolPolicy(cfg.ToolPolicy, cfg.Name)
	a, err := kit.New(ctx, &cfg)
	if err != nil {
		return history, fmt.Errorf("create agentkit session: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, a.ClearCheckpoint(cleanup), a.CloseContext(cleanup))
	}()
	unsubscribe := a.Subscribe(func(event kit.Event) {
		// 根执行错误由 Run 统一发布；成员失败仍保留其独立归属。
		if event.Type != kit.EventError || event.Agent != cfg.Name {
			c.consume(event)
		}
	})
	defer unsubscribe()
	// Continue 保留完整的输入消息，包括多模态和工具结果；讨论的最后一条
	// assistant 消息通过请求级提示转换成下一位成员的输入，不篡改历史。
	if len(history) > 0 && history[len(history)-1].Role == schema.Assistant {
		err = a.Prompt(ctx, "请根据以上讨论继续，给出你的分析与结论。")
	} else {
		err = a.Continue(ctx)
	}
	for err == nil && len(a.PendingInterrupts()) > 0 {
		interrupts := c.interrupts(a.PendingInterrupts())
		decisions := make(runtimeport.InterruptDecisions)
		if opts.InterruptHandler != nil {
			decisions, err = opts.InterruptHandler(ctx, interrupts)
			if decisions == nil {
				decisions = make(runtimeport.InterruptDecisions)
			}
			if err != nil {
				break
			}
		}
		// 未配置处理器或漏答的中断保持拒绝，不能让缺失决策变成授权。
		for _, interrupt := range interrupts {
			if _, ok := decisions[interrupt.ID]; !ok {
				decisions[interrupt.ID] = 0
			}
		}
		err = a.Resume(ctx, decisions)
	}
	return a.History(), err
}

// converter 串行输出模型观察与 AgentKit 工具事件，状态只属于当前 Run。
type converter struct {
	mu         sync.Mutex
	emitter    *events.Emitter
	identities *toolIdentityTracker
	members    map[string]string
	scopes     map[string]MemberScope
	completed  map[string]bool
	cancel     context.CancelFunc
	sinkErr    error
}

func newConverter(emitter *events.Emitter, cancel context.CancelFunc) *converter {
	return &converter{emitter: emitter, identities: newToolIdentityTracker(), members: make(map[string]string), scopes: make(map[string]MemberScope), completed: make(map[string]bool), cancel: cancel}
}

func (c *converter) emit(event events.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emitLocked(event)
}

func (c *converter) emitLocked(event events.Event) error {
	if c.sinkErr != nil {
		return c.sinkErr
	}
	if err := c.emitter.Emit(event); err != nil {
		c.sinkErr = err
		c.cancel()
		return err
	}
	return nil
}

func (c *converter) lastEvent() events.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.emitter.LastEvent()
}
func (c *converter) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sinkErr
}
func (c *converter) registerMembers(members map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, value := range members {
		c.members[key] = value
	}
}
func (c *converter) memberScope(name string) MemberScope {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scopes[name]
}

func (c *converter) consume(event kit.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	scope := c.scopes[event.Agent]
	switch event.Type {
	case kit.EventToolEnd, kit.EventToolUpdate:
		if events.IsInternalToolName(event.ToolName) || events.IsInternalContinueContent(event.Content) {
			return
		}
		meta := events.ToolEvent{AgentName: event.Agent, ToolCallID: event.ToolCallID, ToolName: event.ToolName, ToolArgs: event.ToolArguments, ToolResult: event.Content, Content: event.Content}
		next := events.ToolCallCompleted(meta)
		if event.Type == kit.EventToolUpdate {
			next = events.ToolCallResultDelta(meta)
		}
		scope.apply(&next, c)
		c.identities.attach(&next, scope)
		if event.Type == kit.EventToolEnd {
			if c.completed[next.ToolCallID] {
				return
			}
			c.completed[next.ToolCallID] = true
		}
		_ = c.emitLocked(next)
	case kit.EventError:
		if event.Error != nil && !errors.Is(event.Error, context.Canceled) {
			next := events.Error(event.Agent, "", event.Error)
			scope.apply(&next, c)
			_ = c.emitLocked(next)
		}
	case kit.EventTransfer:
		_ = c.emitLocked(events.SystemNotice(event.Agent, "", domainevent.NoticeTransfer, event.Content))
	}
}

func (c *converter) interrupts(points []kit.InterruptPoint) []runtimeport.Interrupt {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]runtimeport.Interrupt, 0, len(points))
	for _, point := range points {
		next := runtimeport.Interrupt{ID: point.ID, IsRootCause: true, Info: point.Info}
		if payload, ok := point.Info.(runtimeport.InterruptPayload); ok {
			next.Info = payload.Info
			next.MemberCallID, next.MemberToolName, next.MemberName = payload.Metadata.MemberCallID, payload.Metadata.MemberToolName, payload.Metadata.MemberName
			next.MemberOrder = payload.Metadata.MemberOrder
			if next.MemberOrder == nil {
				if order, ok := c.identities.orderForID(next.MemberCallID); ok {
					next.MemberOrder = &order
				}
			}
		}
		result = append(result, next)
	}
	return result
}

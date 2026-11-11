package channel

import (
	"context"
	"fmt"

	"fkteams/internal/adapters/storage/file/history"
	appagent "fkteams/internal/app/agent"
	"fkteams/internal/app/agent/catalog/toolmeta"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/appstate"
	appchat "fkteams/internal/app/chat"
	"fkteams/internal/app/config"
	"fkteams/internal/app/execution"
	appschedule "fkteams/internal/app/schedule"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/approval"
	"fkteams/internal/runtime/events"
	"fkteams/internal/runtime/log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// queuedMessage 队列中的待处理消息
type queuedMessage struct {
	ctx          context.Context
	channelName  string
	chatID       string
	senderID     string
	msg          Message
	isGroup      bool
	userInput    string // 预处理后的用户输入文本
	releaseLease func()
}

// sessionQueue 每个会话的消息队列，确保同一会话的消息串行处理
type sessionQueue struct {
	ch      chan queuedMessage
	pending atomic.Int32 // 队列中待处理的消息数（含正在执行的）
}

const (
	// batchWaitDuration 收到首条消息后等待批量收集的时间窗口
	batchWaitDuration = 500 * time.Millisecond
	// maxBatchSize 单次批量处理的最大消息数
	maxBatchSize = 10
	// sessionQueueBuffer 每个会话队列的缓冲区大小
	sessionQueueBuffer = 50
	// sessionIdleTimeout 会话队列空闲超时，超时后 worker 自动退出
	sessionIdleTimeout = 10 * time.Minute
)

// Bridge 连接通道消息与智能体执行引擎
type Bridge struct {
	executionGate *config.ExecutionGate
	manager       *Manager
	mode          string // 运行模式: team, deep, roundtable, agent
	agentID       string
	state         *appstate.State
	historyDir    string
	sessions      *eventlog.SessionHistoryManager

	runnerMu  sync.Mutex
	runner    runtimeport.Runner
	runnerErr error

	runtimeMu    sync.RWMutex
	dependencies execution.Dependencies
	scheduler    func() *appschedule.Service

	queueMu   sync.Mutex
	queues    map[string]*sessionQueue // per-session 消息队列
	accepting bool
	runCtx    context.Context
	cancelRun context.CancelFunc
	stopCh    chan struct{}
	stopDone  chan struct{}
	workerWG  sync.WaitGroup
}

// BridgeOptions 描述通道桥接器的显式依赖。
type BridgeOptions struct {
	ExecutionGate     *config.ExecutionGate
	State             *appstate.State
	HistoryDir        string
	Sessions          *eventlog.SessionHistoryManager
	SchedulerProvider func() *appschedule.Service
	AgentID           string
}

// NewBridge 创建消息桥接器
func NewBridge(manager *Manager, mode string) *Bridge {
	return NewBridgeWithState(manager, mode, nil)
}

// NewBridgeWithState 创建带应用状态的消息桥接器。
func NewBridgeWithState(manager *Manager, mode string, state *appstate.State) *Bridge {
	return NewBridgeWithOptions(manager, mode, BridgeOptions{State: state})
}

// NewBridgeWithOptions 创建带显式依赖的消息桥接器。
func NewBridgeWithOptions(manager *Manager, mode string, options BridgeOptions) *Bridge {
	if mode == "" {
		mode = "team"
	}
	historyDir := options.HistoryDir
	if historyDir == "" {
		historyDir = appdata.SessionsDir()
	}
	sessions := options.Sessions
	if sessions == nil {
		sessions = eventlog.NewSessionHistoryManager()
	}
	return &Bridge{
		executionGate: options.ExecutionGate,
		manager:       manager,
		mode:          mode,
		agentID:       options.AgentID,
		state:         options.State,
		historyDir:    historyDir,
		sessions:      sessions,
		scheduler:     options.SchedulerProvider,
		queues:        make(map[string]*sessionQueue),
	}
}

// Start 开始接收消息，并为本轮生命周期创建统一取消上下文。
func (b *Bridge) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.queueMu.Lock()
	defer b.queueMu.Unlock()
	if b.accepting {
		return
	}
	if b.stopDone != nil {
		select {
		case <-b.stopDone:
			b.stopDone = nil
		default:
			return
		}
	}
	b.runCtx, b.cancelRun = context.WithCancel(ctx)
	b.stopCh = make(chan struct{})
	b.accepting = true
}

// Stop 停止接收消息，释放未消费消息并等待 worker 退出。
func (b *Bridge) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	b.queueMu.Lock()
	if !b.accepting {
		done := b.stopDone
		b.queueMu.Unlock()
		if done == nil {
			return nil
		}
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("wait for channel workers: %w", ctx.Err())
		}
	}
	b.accepting = false
	b.stopDone = make(chan struct{})
	done := b.stopDone
	if b.cancelRun != nil {
		b.cancelRun()
	}
	close(b.stopCh)
	for sessionID, queue := range b.queues {
		for {
			select {
			case message := <-queue.ch:
				queue.pending.Add(-1)
				releaseQueuedMessage(message)
			default:
				delete(b.queues, sessionID)
				goto drained
			}
		}
	drained:
	}
	b.queueMu.Unlock()

	go func() {
		b.workerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for channel workers: %w", ctx.Err())
	}
}

// ResetRunner 重置 runner，下次请求时会使用新配置重建（配置变更后调用）
func (b *Bridge) ResetRunner() {
	b.runnerMu.Lock()
	defer b.runnerMu.Unlock()
	b.runner = nil
	b.runnerErr = nil
}

// SetRuntimeDependencies 设置当前通道服务实例使用的 runtime 依赖。
func (b *Bridge) SetRuntimeDependencies(dependencies execution.Dependencies) {
	b.runtimeMu.Lock()
	b.dependencies = dependencies
	b.runtimeMu.Unlock()
	b.ResetRunner()
}

func (b *Bridge) withRuntimeContext(ctx context.Context) context.Context {
	b.runtimeMu.RLock()
	dependencies := b.dependencies
	b.runtimeMu.RUnlock()
	ctx = dependencies.Context(ctx)
	if b.scheduler != nil {
		ctx = appschedule.WithService(ctx, b.scheduler())
	}
	return ctx
}

// getRunner 惰性创建 runner（线程安全）
func (b *Bridge) getRunner(ctx context.Context) (runtimeport.Runner, error) {
	b.runnerMu.Lock()
	defer b.runnerMu.Unlock()

	if b.runner != nil || b.runnerErr != nil {
		return b.runner, b.runnerErr
	}

	b.runner, b.runnerErr = appagent.Resolve(ctx, b.mode, b.agentID)
	return b.runner, b.runnerErr
}

// HandleMessage 处理来自通道的消息，入队后由 per-session worker 串行执行
func (b *Bridge) HandleMessage(ctx context.Context, chatID, senderID string, msg Message, isGroup bool) {
	channelName := "unknown"
	if name, ok := ctx.Value(channelNameKey{}).(string); ok {
		channelName = name
	}

	userInput := buildUserInput(msg)
	if userInput == "" {
		return
	}

	sessionID := fmt.Sprintf("channel_%s_%s", channelName, chatID)

	qm := queuedMessage{
		ctx:         ctx,
		channelName: channelName,
		chatID:      chatID,
		senderID:    senderID,
		msg:         msg,
		isGroup:     isGroup,
		userInput:   userInput,
	}

	b.queueMu.Lock()
	if !b.accepting {
		b.queueMu.Unlock()
		return
	}
	b.queueMu.Unlock()
	qm.releaseLease = eventlog.AcquireSessionLease(b.historyDir, sessionID)

	b.queueMu.Lock()
	if !b.accepting {
		b.queueMu.Unlock()
		releaseQueuedMessage(qm)
		return
	}
	q, exists := b.queues[sessionID]
	if !exists {
		q = &sessionQueue{ch: make(chan queuedMessage, sessionQueueBuffer)}
		b.queues[sessionID] = q
		b.workerWG.Add(1)
		go b.sessionWorker(b.runCtx, b.stopCh, sessionID, q)
	}
	pos, accepted := enqueueSessionMessage(q, qm)
	b.queueMu.Unlock()

	if accepted {
		// 队列中有其他消息排队时通知用户位置和批次
		if pos > 1 {
			batchNum := (pos-1)/maxBatchSize + 1
			notice := fmt.Sprintf("消息已加入队列（第 %d 位），预计在第 %d 批执行，前面还有 %d 条消息在处理中", pos, batchNum, pos-1)
			_ = b.manager.SendText(ctx, channelName, chatID, notice)
		}
		return
	}
	log.Printf("[bridge] session queue full, dropping message: session=%s", sessionID)
	qm.releaseLease()
	_ = b.manager.SendText(ctx, channelName, chatID, "消息队列已满，请稍后再试")
}

// enqueueSessionMessage 必须在持有 Bridge.queueMu 时调用，保证空闲 worker
// 不能在取出队列和发布消息之间删除该队列。
func enqueueSessionMessage(q *sessionQueue, msg queuedMessage) (int, bool) {
	pos := int(q.pending.Add(1))
	select {
	case q.ch <- msg:
		return pos, true
	default:
		q.pending.Add(-1)
		return 0, false
	}
}

// sessionWorker 每个会话的消息处理协程，批量取出消息后串行执行
// 空闲超时后自动退出并清理队列
func (b *Bridge) sessionWorker(runCtx context.Context, stopCh <-chan struct{}, sessionID string, q *sessionQueue) {
	defer b.workerWG.Done()
	for {
		// 阻塞等待第一条消息，带空闲超时
		var first queuedMessage
		var ok bool
		idleTimer := time.NewTimer(sessionIdleTimeout)
		select {
		case first, ok = <-q.ch:
			idleTimer.Stop()
			if !ok {
				return
			}
		case <-stopCh:
			idleTimer.Stop()
			return
		case <-idleTimer.C:
			// 空闲超时：加锁后再次检查是否有新消息，避免竞态丢消息
			b.queueMu.Lock()
			select {
			case first, ok = <-q.ch:
				b.queueMu.Unlock()
				if !ok {
					return
				}
				// 有新消息，继续处理
			default:
				// 确认无消息，安全退出
				delete(b.queues, sessionID)
				b.queueMu.Unlock()
				return
			}
		}

		// 收到首条消息后短暂等待，收集同一时间段内的更多消息
		batch := []queuedMessage{first}
		timer := time.NewTimer(batchWaitDuration)
		stopping := false
	collect:
		for len(batch) < maxBatchSize {
			select {
			case msg, ok := <-q.ch:
				if !ok {
					break collect
				}
				batch = append(batch, msg)
			case <-timer.C:
				break collect
			case <-stopCh:
				stopping = true
				break collect
			}
		}
		timer.Stop()

		if stopping {
			for _, message := range batch {
				releaseQueuedMessage(message)
			}
			q.pending.Add(-int32(len(batch)))
			return
		}
		b.processBatch(runCtx, sessionID, batch)
		q.pending.Add(-int32(len(batch)))
	}
}

// processBatch 处理一批消息：合并用户输入，通知用户，执行引擎
func (b *Bridge) processBatch(runCtx context.Context, sessionID string, batch []queuedMessage) {
	releaseExecution := b.executionGate.Begin()
	defer releaseExecution()
	defer func() {
		for _, message := range batch {
			releaseQueuedMessage(message)
		}
	}()
	first := batch[0]
	channelName := first.channelName
	chatID := first.chatID

	// 使用独立 context：入队消息的原始 ctx 可能已被取消（如 typing ctx）
	ctx := WithChannelName(runCtx, channelName)
	ctx = appstate.WithState(ctx, b.state)
	ctx = b.withRuntimeContext(ctx)

	r, err := b.getRunner(ctx)
	if err != nil {
		log.Printf("[bridge] create runner failed: %v", err)
		_ = b.manager.SendText(ctx, channelName, chatID, "internal error: "+err.Error())
		return
	}

	// 合并所有消息为一次输入
	var combinedInput string
	if len(batch) == 1 {
		combinedInput = first.userInput
	} else {
		// 多条消息：通知用户将要执行的任务列表
		var preview strings.Builder
		fmt.Fprintf(&preview, "收到 %d 条消息，将依次处理：", len(batch))
		for i, m := range batch {
			line := m.userInput
			if len([]rune(line)) > 50 {
				line = string([]rune(line)[:50]) + "..."
			}
			fmt.Fprintf(&preview, "\n%d. %s", i+1, line)
		}
		_ = b.manager.SendText(ctx, channelName, chatID, preview.String())

		// 合并为带编号的用户输入
		var merged strings.Builder
		merged.WriteString("以下是用户连续发送的多条消息，请依次处理每一条：\n\n")
		for i, m := range batch {
			fmt.Fprintf(&merged, "--- 消息 %d ---\n%s\n\n", i+1, m.userInput)
		}
		combinedInput = merged.String()
	}

	recorder, releaseRecorder := b.sessions.Acquire(sessionID, b.historyDir)
	defer releaseRecorder()
	recorder.SetToolDisplayResolver(toolmeta.ResolverFromContext(ctx))
	turnInput := appchat.BuildTurnInputWithMemory(recorder, combinedInput, b.memoryManager())

	rc := newReplyCollector(b.manager, channelName, chatID)

	_, runErr := appchat.NewService().RunTurn(ctx, appchat.TurnRequest{
		SessionID:        sessionID,
		Runner:           r,
		Input:            turnInput,
		Summary:          recorder,
		History:          recorder,
		NonInteractive:   true,
		ApprovalRegistry: approval.NewAutoApproveRegistry(),
		EventSink:        rc.handleEvent,
		OnFinish: func(ctx context.Context, _ *runtimeport.RunResult, err error) error {
			rc.flush()
			store := eventlog.NewChatSessionStore(b.historyDir, b.sessions.Documents)
			lifecycleErr := appchat.NewSessionLifecycle(store).Finish(ctx, appchat.FinishRequest{
				SessionID:      sessionID,
				TitleSource:    combinedInput,
				DefaultTitle:   "通道会话",
				Error:          err,
				Memory:         b.memoryManager(),
				MemoryMessages: eventlog.ConvertMemoryMessages(recorder),
			})
			if !rc.replied {
				_ = b.manager.SendText(ctx, channelName, chatID, "...")
			}
			return lifecycleErr
		},
	})
	if runErr != nil {
		log.Printf("[bridge] task failed: session=%s, err=%v", sessionID, runErr)
	}
}

func releaseQueuedMessage(message queuedMessage) {
	if message.releaseLease != nil {
		message.releaseLease()
	}
}

func (b *Bridge) memoryManager() appstate.MemoryManager {
	if b == nil || b.state == nil {
		return nil
	}
	return b.state.Memory()
}

// buildUserInput 将消息内容和附件构建为用户输入文本
func buildUserInput(msg Message) string {
	userInput := msg.Content
	for _, att := range msg.Attachments {
		desc := att.TypeName() + ": " + att.URL
		if att.FileName != "" {
			desc = att.TypeName() + " (" + att.FileName + "): " + att.URL
		}
		if userInput != "" {
			userInput += "\n"
		}
		userInput += "[" + desc + "]"
	}
	return userInput
}

// replyCollector 收集并发送助手回复（支持流式和非流式）
type replyCollector struct {
	manager     *Manager
	channelName string
	chatID      string

	mu           sync.Mutex
	pendingParts []string                   // 当前智能体的累积流式文本
	pendingCalls map[string]pendingToolCall // 待匹配结果的工具调用（按 ID 索引）
	resultChunks map[string]string          // 待合并的流式工具结果
	currentAgent string                     // 当前流式响应的智能体
	replied      bool                       // 是否已发送过任何回复
}

// pendingToolCall 等待结果的工具调用
type pendingToolCall struct {
	name string
	args string
}

func newReplyCollector(mgr *Manager, channelName, chatID string) *replyCollector {
	return &replyCollector{
		manager:      mgr,
		channelName:  channelName,
		chatID:       chatID,
		pendingCalls: make(map[string]pendingToolCall),
		resultChunks: make(map[string]string),
	}
}

// handleEvent 处理引擎产生的各类事件
func (rc *replyCollector) handleEvent(event events.Event) error {
	switch event.Type {
	case events.EventAssistantText:
		if event.DeltaKind != "" && event.DeltaKind != events.DeltaOutput {
			return nil
		}
		// 流式文本块：累积，检测到 agent 切换时 flush
		rc.flushToolResultChunks()
		rc.mu.Lock()
		if event.AgentName != rc.currentAgent && rc.currentAgent != "" {
			rc.mu.Unlock()
			rc.flush()
			rc.mu.Lock()
		}
		rc.currentAgent = event.AgentName
		if event.Content != "" {
			rc.pendingParts = append(rc.pendingParts, event.Content)
		}
		rc.mu.Unlock()
	case events.EventSystemNotice:
		if event.Notice != nil && event.Notice.Code == "transfer" {
			rc.flush()
		}
	case events.EventToolCallStarted:
		// 工具调用：flush 之前的文本，按 ToolCall.ID 记录所有工具调用
		rc.flush()
		rc.mu.Lock()
		for _, tc := range events.ToolCallsFromEvent(event) {
			rc.pendingCalls[tc.ID] = pendingToolCall{
				name: tc.Function.Name,
				args: truncateText(tc.Function.Arguments, 200),
			}
		}
		rc.mu.Unlock()
	case events.EventToolCallCompleted:
		// 工具调用完成：按 ToolCallID 匹配调用，发送摘要
		rc.mu.Lock()
		call, found := rc.pendingCalls[event.ToolCallID]
		if found {
			delete(rc.pendingCalls, event.ToolCallID)
		}
		content := event.Content
		if chunked := rc.resultChunks[event.ToolCallID]; chunked != "" {
			if content == "" || strings.Contains(chunked, content) {
				content = chunked
			} else {
				content = chunked + content
			}
			delete(rc.resultChunks, event.ToolCallID)
		}
		rc.mu.Unlock()
		if found {
			result := truncateText(content, 200)
			summary := "[" + call.name + "] " + call.args + "\n-> " + result
			rc.send(summary)
		}
	case events.EventToolCallResult:
		rc.mu.Lock()
		if event.ToolCallID != "" {
			rc.resultChunks[event.ToolCallID] += event.Content
		}
		rc.mu.Unlock()
	}
	return nil
}

func (rc *replyCollector) flushToolResultChunks() {
	rc.mu.Lock()
	items := make([]struct {
		call   pendingToolCall
		result string
	}, 0)
	for callID, content := range rc.resultChunks {
		call, found := rc.pendingCalls[callID]
		delete(rc.resultChunks, callID)
		if found {
			delete(rc.pendingCalls, callID)
			items = append(items, struct {
				call   pendingToolCall
				result string
			}{call: call, result: content})
		}
	}
	rc.mu.Unlock()
	for _, item := range items {
		result := truncateText(item.result, 200)
		summary := "[" + item.call.name + "] " + item.call.args + "\n-> " + result
		rc.send(summary)
	}
}

// flush 发送累积的流式文本
func (rc *replyCollector) flush() {
	rc.mu.Lock()
	text := strings.TrimSpace(strings.Join(rc.pendingParts, ""))
	rc.pendingParts = rc.pendingParts[:0]
	rc.mu.Unlock()
	rc.send(text)
}

// truncateText 截断文本，保留前 maxLen 个字符
func truncateText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// send 发送文本消息（自动分片）
func (rc *replyCollector) send(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	rc.replied = true
	ctx := context.Background()
	for _, chunk := range splitMessage(text, 2000) {
		if err := rc.manager.SendText(ctx, rc.channelName, rc.chatID, chunk); err != nil {
			log.Printf("[bridge] send reply failed: channel=%s, chat=%s, err=%v", rc.channelName, rc.chatID, err)
			break
		}
	}
}

// channelNameKey 是通道名称的 context key。
type channelNameKey struct{}

// WithChannelName 将通道名称注入 context
func WithChannelName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, channelNameKey{}, name)
}

// splitMessage 按最大长度分割消息，优先在换行处分割以保持语义完整
func splitMessage(text string, maxLen int) []string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(runes) > 0 {
		end := maxLen
		if end > len(runes) {
			end = len(runes)
		}
		// 在 maxLen 范围内查找最后一个换行，优先在换行处分割
		if end < len(runes) {
			best := -1
			for i := end - 1; i >= end/2; i-- {
				if runes[i] == '\n' {
					best = i + 1 // 包含换行符
					break
				}
			}
			if best > 0 {
				end = best
			}
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}

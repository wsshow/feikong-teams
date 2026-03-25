package channels

import (
	"context"
	"fkteams/agents"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/log"
	"fkteams/runner"
	"fkteams/tools/approval"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
)

// queuedMessage 队列中的待处理消息
type queuedMessage struct {
	ctx         context.Context
	channelName string
	chatID      string
	senderID    string
	msg         Message
	isGroup     bool
	userInput   string // 预处理后的用户输入文本
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
	// channelHistoryDir 通道会话历史存储目录，与 Web/CLI 共用
	channelHistoryDir = "sessions/"
)

// Bridge 连接通道消息与智能体执行引擎
type Bridge struct {
	manager *Manager
	mode    string // 运行模式: team, deep, roundtable, custom 或智能体名称

	runnerOnce sync.Once
	runner     *adk.Runner
	runnerErr  error

	queueMu sync.Mutex
	queues  map[string]*sessionQueue // per-session 消息队列
}

// NewBridge 创建消息桥接器
func NewBridge(manager *Manager, mode string) *Bridge {
	if mode == "" {
		mode = "team"
	}
	return &Bridge{
		manager: manager,
		mode:    mode,
		queues:  make(map[string]*sessionQueue),
	}
}

// getRunner 惰性创建 runner（线程安全）
func (b *Bridge) getRunner(ctx context.Context) (*adk.Runner, error) {
	b.runnerOnce.Do(func() {
		switch b.mode {
		case "team":
			b.runner, b.runnerErr = runner.CreateSupervisorRunner(ctx)
		case "roundtable":
			b.runner, b.runnerErr = runner.CreateLoopAgentRunner(ctx)
		case "custom":
			b.runner, b.runnerErr = runner.CreateCustomSupervisorRunner(ctx)
		case "deep":
			b.runner, b.runnerErr = runner.CreateDeepAgentsRunner(ctx)
		default:
			// 尝试按名称查找单个智能体
			info := agents.GetAgentByName(b.mode)
			if info != nil {
				b.runner = runner.CreateAgentRunner(ctx, info.Creator(ctx))
			} else {
				b.runnerErr = fmt.Errorf("unknown mode or agent: %s", b.mode)
			}
		}
	})
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
	q, exists := b.queues[sessionID]
	if !exists {
		q = &sessionQueue{ch: make(chan queuedMessage, sessionQueueBuffer)}
		b.queues[sessionID] = q
		go b.sessionWorker(sessionID, q)
	}
	b.queueMu.Unlock()

	select {
	case q.ch <- qm:
		pos := int(q.pending.Add(1))
		// 队列中有其他消息排队时通知用户位置和批次
		if pos > 1 {
			batchNum := (pos-1)/maxBatchSize + 1
			notice := fmt.Sprintf("消息已加入队列（第 %d 位），预计在第 %d 批执行，前面还有 %d 条消息在处理中", pos, batchNum, pos-1)
			_ = b.manager.SendText(ctx, channelName, chatID, notice)
		}
	default:
		log.Printf("[bridge] session queue full, dropping message: session=%s", sessionID)
		_ = b.manager.SendText(ctx, channelName, chatID, "消息队列已满，请稍后再试")
	}
}

// sessionWorker 每个会话的消息处理协程，批量取出消息后串行执行
// 空闲超时后自动退出并清理队列
func (b *Bridge) sessionWorker(sessionID string, q *sessionQueue) {
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
			}
		}
		timer.Stop()

		b.processBatch(sessionID, batch)
		q.pending.Add(-int32(len(batch)))
	}
}

// processBatch 处理一批消息：合并用户输入，通知用户，执行引擎
func (b *Bridge) processBatch(sessionID string, batch []queuedMessage) {
	first := batch[0]
	ctx := first.ctx
	channelName := first.channelName
	chatID := first.chatID

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
		preview.WriteString(fmt.Sprintf("收到 %d 条消息，将依次处理：", len(batch)))
		for i, m := range batch {
			line := m.userInput
			if len([]rune(line)) > 50 {
				line = string([]rune(line)[:50]) + "..."
			}
			preview.WriteString(fmt.Sprintf("\n%d. %s", i+1, line))
		}
		_ = b.manager.SendText(ctx, channelName, chatID, preview.String())

		// 合并为带编号的用户输入
		var merged strings.Builder
		merged.WriteString("以下是用户连续发送的多条消息，请依次处理每一条：\n\n")
		for i, m := range batch {
			merged.WriteString(fmt.Sprintf("--- 消息 %d ---\n%s\n\n", i+1, m.userInput))
		}
		combinedInput = merged.String()
	}

	recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, channelHistoryDir)
	messages := chatutil.BuildInputMessages(recorder, combinedInput)
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(combinedInput)

	rc := newReplyCollector(b.manager, channelName, chatID)

	ctx = fkevent.WithNonInteractive(ctx)
	ctx = fkevent.WithCallback(ctx, rc.handleEvent)
	ctx = summary.WithSummaryPersistCallback(ctx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})
	ctx = approval.WithRegistry(ctx, approval.NewAutoApproveRegistry())

	_, err = engine.New(r, sessionID).Run(ctx, messages, engine.WithInterruptHandler(engine.AutoRejectHandler()))
	if err != nil {
		log.Printf("[bridge] run error: session=%s, err=%v", sessionID, err)
	}

	rc.flush()

	// 持久化会话历史和元数据
	historyFile := filepath.Join(channelHistoryDir, sessionID, "history.json")
	if err := recorder.SaveToFile(historyFile); err != nil {
		log.Printf("[bridge] save history failed: session=%s, err=%v", sessionID, err)
	}
	saveChannelSessionMetadata(sessionID, combinedInput)

	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}

	if !rc.replied {
		_ = b.manager.SendText(ctx, channelName, chatID, "...")
	}
}

// saveChannelSessionMetadata 保存通道会话的元数据
func saveChannelSessionMetadata(sessionID, userInput string) {
	sessionDir := filepath.Join(channelHistoryDir, sessionID)
	now := time.Now()
	meta, err := fkevent.LoadMetadata(sessionDir)
	if err != nil {
		title := userInput
		runes := []rune(title)
		if len(runes) > 50 {
			title = string(runes[:50]) + "..."
		}
		if title == "" {
			title = "通道会话"
		}
		meta = &fkevent.SessionMetadata{
			ID:        sessionID,
			Title:     title,
			Status:    "completed",
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		meta.UpdatedAt = now
		meta.Status = "completed"
	}
	if err := fkevent.SaveMetadata(sessionDir, meta); err != nil {
		log.Printf("[bridge] save metadata failed: session=%s, err=%v", sessionID, err)
	}
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
	}
}

// handleEvent 处理引擎产生的各类事件
func (rc *replyCollector) handleEvent(event fkevent.Event) error {
	switch event.Type {
	case "message":
		// 非流式完整消息：先 flush 累积文本，再直接发送
		rc.flush()
		rc.send(event.Content)
	case "stream_chunk":
		// 流式文本块：累积，检测到 agent 切换时 flush
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
	case "action":
		// 智能体切换时 flush
		if event.ActionType == "transfer" {
			rc.flush()
		}
	case "tool_calls":
		// 工具调用：flush 之前的文本，按 ToolCall.ID 记录所有工具调用
		rc.flush()
		rc.mu.Lock()
		for _, tc := range event.ToolCalls {
			rc.pendingCalls[tc.ID] = pendingToolCall{
				name: tc.Function.Name,
				args: truncateText(tc.Function.Arguments, 200),
			}
		}
		rc.mu.Unlock()
	case "tool_calls_preparing":
		rc.flush()
	case "tool_result":
		// 工具调用完成：按 ToolCallID 匹配调用，发送摘要
		rc.mu.Lock()
		call, found := rc.pendingCalls[event.ToolCallID]
		if found {
			delete(rc.pendingCalls, event.ToolCallID)
		}
		rc.mu.Unlock()
		if found {
			result := truncateText(event.Content, 200)
			summary := "[" + call.name + "] " + call.args + "\n-> " + result
			rc.send(summary)
		}
	}
	return nil
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

// channelNameKey context key for channel name
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

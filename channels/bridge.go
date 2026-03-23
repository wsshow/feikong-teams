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
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
)

// Bridge 连接通道消息与智能体执行引擎
type Bridge struct {
	manager *Manager
	mode    string // 运行模式: team, deep, roundtable, custom 或智能体名称

	runnerOnce sync.Once
	runner     *adk.Runner
	runnerErr  error
}

// NewBridge 创建消息桥接器
func NewBridge(manager *Manager, mode string) *Bridge {
	if mode == "" {
		mode = "team"
	}
	return &Bridge{
		manager: manager,
		mode:    mode,
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

// HandleMessage 处理来自通道的消息，执行智能体并将结果通过通道回复
func (b *Bridge) HandleMessage(ctx context.Context, chatID, senderID string, msg Message, isGroup bool) {
	channelName := "unknown"
	if name, ok := ctx.Value(channelNameKey{}).(string); ok {
		channelName = name
	}

	sessionID := fmt.Sprintf("channel_%s_%s", channelName, chatID)

	r, err := b.getRunner(ctx)
	if err != nil {
		log.Printf("[bridge] create runner failed: %v", err)
		_ = b.manager.SendText(ctx, channelName, chatID, "internal error: "+err.Error())
		return
	}

	// 构建用户输入（文本 + 附件描述）
	userInput := buildUserInput(msg)
	if userInput == "" {
		return
	}

	recorder := fkevent.GlobalSessionManager.GetOrCreate(sessionID, "")
	messages := chatutil.BuildInputMessages(recorder, userInput)
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(userInput)

	rc := newReplyCollector(b.manager, channelName, chatID)

	ctx = fkevent.WithNonInteractive(ctx)
	ctx = fkevent.WithCallback(ctx, rc.handleEvent)
	ctx = summary.WithSummaryPersistCallback(ctx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})
	ctx = approval.WithRegistry(ctx, approval.NewRegistry(
		approval.StoreConfig{Name: approval.StoreCommand},
		approval.StoreConfig{Name: approval.StoreFile, Matcher: approval.DirMatchFunc},
		approval.StoreConfig{Name: approval.StoreDispatch},
	))

	_, err = engine.New(r, sessionID).Run(ctx, messages, engine.WithInterruptHandler(engine.AutoRejectHandler()))
	if err != nil {
		log.Printf("[bridge] run error: session=%s, err=%v", sessionID, err)
	}

	rc.flush()

	if g.MemoryManager != nil {
		g.MemoryManager.ExtractFromRecorder(recorder, sessionID)
	}

	if !rc.replied {
		_ = b.manager.SendText(ctx, channelName, chatID, "...")
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
	pendingParts []string // 当前智能体的累积流式文本
	currentAgent string   // 当前流式响应的智能体
	replied      bool     // 是否已发送过任何回复
}

func newReplyCollector(mgr *Manager, channelName, chatID string) *replyCollector {
	return &replyCollector{manager: mgr, channelName: channelName, chatID: chatID}
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
	case "tool_calls_preparing", "tool_calls":
		// 工具调用前 flush 模型输出的文本
		rc.flush()
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

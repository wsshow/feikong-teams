// Package cli 提供 CLI 交互模式的会话管理、查询执行和信号处理
package runtime

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fkteams/appstate"
	"fkteams/events"
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appdata"
	appchat "fkteams/internal/app/chat"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/turn"
	"fkteams/report"
	"fkteams/tools/approval"
	"fkteams/tools/ask"
	"fkteams/tui"

	"github.com/pterm/pterm"
)

// QueryState 查询状态管理
type QueryState struct {
	running    atomic.Bool        // 查询是否在运行
	cancelling atomic.Bool        // 是否正在取消查询
	cancelFunc context.CancelFunc // 查询的取消函数
	cancelMu   sync.Mutex         // 保护 cancelFunc
}

// NewQueryState 创建查询状态管理器
func NewQueryState() *QueryState {
	return &QueryState{}
}

// IsRunning 查询是否正在运行
func (s *QueryState) IsRunning() bool {
	return s.running.Load()
}

// IsCancelling 是否正在取消
func (s *QueryState) IsCancelling() bool {
	return s.cancelling.Load()
}

// SetCancelFunc 设置取消函数
func (s *QueryState) SetCancelFunc(cancel context.CancelFunc) {
	s.cancelMu.Lock()
	s.cancelFunc = cancel
	s.cancelMu.Unlock()
}

// StartQuery 开始查询
func (s *QueryState) StartQuery() {
	s.running.Store(true)
	s.cancelling.Store(false)
}

// EndQuery 结束查询
func (s *QueryState) EndQuery() {
	s.running.Store(false)
	s.cancelling.Store(false)
}

// Cancel 取消当前查询
func (s *QueryState) Cancel() bool {
	if !s.running.Load() {
		return false
	}
	if s.cancelling.Load() {
		return false
	}
	s.cancelling.Store(true)
	s.cancelMu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	s.cancelMu.Unlock()
	return true
}

// QueryExecutor 查询执行器
type QueryExecutor struct {
	state         *QueryState
	runner        runtimeport.Runner
	autoReject    bool
	approveStores []string // 自动批准的 store 列表
	view          QueryView
	askRuntime    ask.RuntimeHandler
	steeringMu    sync.Mutex
	steering      []domainmessage.Message
	memory        appstate.MemoryManager
}

// NewQueryExecutor 创建查询执行器
func NewQueryExecutor(runner runtimeport.Runner, state *QueryState) *QueryExecutor {
	return &QueryExecutor{
		state:  state,
		runner: runner,
		view:   NewTerminalQueryView(),
	}
}

// SetAutoReject 设置自动拒绝危险命令（用于非交互模式）
func (e *QueryExecutor) SetAutoReject(v bool) {
	e.autoReject = v
}

// SetApproveStores 设置自动批准的操作类别（逗号分隔: all/command/file/dispatch）
func (e *QueryExecutor) SetApproveStores(s string) {
	if s == "" {
		return
	}
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			e.approveStores = append(e.approveStores, v)
		}
	}
}

// SetRunner 设置 runner
func (e *QueryExecutor) SetRunner(runner runtimeport.Runner) {
	e.runner = runner
}

// SetMemoryManager 设置执行器使用的长期记忆管理器。
func (e *QueryExecutor) SetMemoryManager(manager appstate.MemoryManager) {
	e.memory = manager
}

// SetCallbackBuilder 设置事件回调构造器，用于 JSON 等非默认输出格式。
func (e *QueryExecutor) SetCallbackBuilder(cb func(*eventlog.HistoryRecorder) func(events.Event) error) {
	e.view = callbackQueryView{callbackBuilder: cb}
}

// SetView 设置查询输出视图。
func (e *QueryExecutor) SetView(view QueryView) {
	if view != nil {
		e.view = view
	}
}

// SetAskRuntimeHandler 设置运行时 ask_questions 处理器。
func (e *QueryExecutor) SetAskRuntimeHandler(handler ask.RuntimeHandler) {
	e.askRuntime = handler
}

func (e *QueryExecutor) QueueSteering(input string) bool {
	if strings.TrimSpace(input) == "" || !e.state.IsRunning() {
		return false
	}
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	e.steering = append(e.steering, domainmessage.Message{Role: domainmessage.RoleUser, Content: input})
	return true
}

func (e *QueryExecutor) SteeringQueueSnapshot() []domainmessage.Message {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	messages := make([]domainmessage.Message, len(e.steering))
	copy(messages, e.steering)
	return messages
}

func (e *QueryExecutor) takeSteeringMessages(limit int) []domainmessage.Message {
	e.steeringMu.Lock()
	defer e.steeringMu.Unlock()
	if len(e.steering) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(e.steering) {
		limit = len(e.steering)
	}
	messages := make([]domainmessage.Message, limit)
	copy(messages, e.steering[:limit])
	e.steering = e.steering[limit:]
	return messages
}

func (e *QueryExecutor) drainSteeringMessages() []domainmessage.Message {
	return e.takeSteeringMessages(0)
}

func (e *QueryExecutor) drainSteeringMessage() (domainmessage.Message, bool) {
	messages := e.drainSteeringMessages()
	if len(messages) == 0 {
		return domainmessage.Message{}, false
	}
	return mergeSteeringMessages(messages), true
}

func (e *QueryExecutor) DrainSteeringText() string {
	messages := e.drainSteeringMessages()
	if len(messages) == 0 {
		return ""
	}
	return strings.TrimSpace(mergeSteeringMessages(messages).DisplayText())
}

func mergeSteeringMessages(messages []domainmessage.Message) domainmessage.Message {
	if len(messages) == 1 {
		return messages[0]
	}
	var sb strings.Builder
	for i, message := range messages {
		text := strings.TrimSpace(message.DisplayText())
		if text == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "%d. %s", i+1, text)
	}
	return domainmessage.Message{Role: domainmessage.RoleUser, Content: sb.String()}
}

// CLI 模式会话常量
const (
	CLISessionID = "cli"
)

// CLIHistoryDir CLI 会话历史存储目录
var CLIHistoryDir = appdata.SessionsDir()

// activeSessionID 当前活跃的会话 ID，每次启动时生成新 ID
var activeSessionID = CLISessionID

// cliSessionTitle 缓存第一次用户输入作为会话标题，仅在保存时使用
var cliSessionTitle string

// resumeSessionID 恢复会话的 ID，由 -r 参数设置
var resumeSessionID string

// temporarySession 标记当前 CLI 会话不持久化。
var temporarySession bool

// NewDirectSessionID 生成基于 UUID 的唯一会话 ID
func NewDirectSessionID() string {
	return session.NewID()
}

// SetResumeSessionID 设置要恢复的会话 ID
func SetResumeSessionID(sessionID string) {
	resumeSessionID = sessionID
}

// SetTemporarySession 设置当前 CLI 是否为临时会话。
func SetTemporarySession(v bool) {
	temporarySession = v
}

// IsTemporarySession 返回当前 CLI 是否为临时会话。
func IsTemporarySession() bool {
	return temporarySession
}

// getCliRecorder 获取 CLI 模式的历史记录器
func getCliRecorder() *eventlog.HistoryRecorder {
	return eventlog.GlobalSessionManager.GetOrCreate(activeSessionID, CLIHistoryDir)
}

// BuildTurnInput 构建一轮输入（包含历史对话，支持上下文压缩摘要）
func BuildTurnInput(input string) domainmessage.TurnInput {
	recorder := getCliRecorder()
	return appchat.BuildTurnInput(recorder, input)
}

// BuildTurnInputWithMemory 构建包含显式长期记忆依赖的一轮输入。
func BuildTurnInputWithMemory(input string, manager appstate.MemoryManager) domainmessage.TurnInput {
	recorder := getCliRecorder()
	return appchat.BuildTurnInputWithMemory(recorder, input, manager)
}

// Execute 执行查询
func (e *QueryExecutor) Execute(ctx context.Context, input string) error {
	turnInput := BuildTurnInputWithMemory(input, e.memory)
	recorder := getCliRecorder()

	// 缓存第一次输入作为会话标题（不立即创建文件，等用户保存时才写入）
	if cliSessionTitle == "" {
		cliSessionTitle = truncateTitle(input)
	}

	// 创建可取消的 context
	queryCtx, cancelFunc := context.WithCancel(ctx)

	e.view.Start(input)
	innerCallback := e.view.EventCallback(recorder)

	var approvalReg *approval.Registry
	if len(e.approveStores) > 0 {
		approvalReg = approval.NewDefaultSelectiveRegistry(e.approveStores)
	} else {
		approvalReg = approval.NewDefaultRegistry()
	}

	var handler turn.InterruptHandler
	if !e.autoReject {
		handler = turn.InfoHandler(func(info any) (any, bool) {
			if askInfo, ok := info.(*ask.AskInfo); ok {
				return e.promptAskQuestions(askInfo), true
			}
			return e.promptApproval(), true
		})
	}

	e.state.SetCancelFunc(cancelFunc)
	e.state.StartQuery()
	defer e.state.EndQuery()

	startTime := time.Now()
	currentInput := turnInput
	steeringSource := func(context.Context) ([]domainmessage.Message, error) {
		message, ok := e.drainSteeringMessage()
		if !ok {
			return nil, nil
		}
		recorder.RecordUserMessage(message)
		if err := innerCallback(events.Event{
			Type:    events.EventType(events.NotifyProcessingStart),
			Content: strings.TrimSpace(message.DisplayText()),
			Detail:  "steering",
		}); err != nil {
			return nil, err
		}
		return []domainmessage.Message{message}, nil
	}
	for {
		_, err := appchat.NewService().RunTurn(queryCtx, appchat.TurnRequest{
			SessionID: activeSessionID,
			Runner:    e.runner,
			Input:     currentInput,
		},
			appchat.OnEvent(func(event events.Event) error {
				return innerCallback(event)
			}),
			appchat.WithHistory(recorder),
			appchat.OnInterrupt(runtimeport.InterruptHandler(handler)),
			appchat.WithContext(approval.RegistryContext(approvalReg)),
			appchat.WithContext(func(ctx context.Context) context.Context {
				return runtimeport.WithSteeringSource(ctx, steeringSource)
			}),
			appchat.WithContext(func(ctx context.Context) context.Context {
				return ask.WithRuntimeHandler(ctx, e.askRuntime)
			},
			),
		)

		recorder.FinalizeCurrent()
		if err != nil {
			if queryCtx.Err() != nil {
				e.view.Interrupted()
				return nil
			}
			e.view.Error(err)
			return nil
		}

		if next, ok := e.drainSteeringMessage(); ok && queryCtx.Err() == nil {
			currentInput = appchat.BuildTurnInputWithMemory(recorder, next.DisplayText(), e.memory)
			continue
		}
		break
	}

	if queryCtx.Err() == nil {
		e.view.Flush()
	}

	if e.memory != nil {
		e.memory.ExtractFromRecorder(recorder, activeSessionID)
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	e.view.Done(elapsed)
	return nil
}

// promptApproval 提示用户审批，返回审批决定
func (e *QueryExecutor) promptApproval() int {
	if e.autoReject {
		e.view.AutoReject()
		return approval.Reject
	}

	fmt.Println()
	options := []string{
		"允许一次",
		"该会话允许该项",
		"该会话允许所有",
		"拒绝执行",
	}
	selected, _ := pterm.DefaultInteractiveSelect.
		WithDefaultText("请选择操作").
		WithOptions(options).
		Show()
	fmt.Println()

	switch selected {
	case "允许一次":
		return approval.ApproveOnce
	case "该会话允许该项":
		return approval.ApproveItem
	case "该会话允许所有":
		return approval.ApproveAll
	default:
		return approval.Reject
	}
}

// promptAskQuestions 在 CLI 中展示问题并收集用户回答
func (e *QueryExecutor) promptAskQuestions(info *ask.AskInfo) *ask.AskResponse {
	var options []tui.AskOption
	for _, opt := range info.Options {
		options = append(options, tui.AskOption{Label: opt, Value: opt})
	}

	result, err := tui.AskQuestions(info.Question, options, info.MultiSelect)
	if err != nil || result == nil {
		return &ask.AskResponse{}
	}
	return &ask.AskResponse{
		Selected: result.Selected,
		FreeText: result.FreeText,
	}
}

// HandleCtrlC 处理 Ctrl+C 事件，只中断查询，不退出程序
func HandleCtrlC(state *QueryState) {
	if !state.running.Load() {
		return // 空闲状态，忽略
	}

	// 防止重复中断
	if state.cancelling.Load() {
		return
	}
	state.cancelling.Store(true)
	state.cancelMu.Lock()
	if state.cancelFunc != nil {
		state.cancelFunc()
	}
	state.cancelMu.Unlock()
}

// SaveChatHistoryToHTML 保存聊天历史到 HTML 文件
func SaveChatHistoryToHTML() (string, error) {
	recorder := getCliRecorder()
	filePath, err := recorder.SaveToMarkdownWithTimestamp()
	if err != nil {
		return "", fmt.Errorf("保存聊天历史到 Markdown 失败: %w", err)
	}
	htmlFilePath, err := report.ConvertMarkdownFileToNiceHTMLFile(filePath)
	if err != nil {
		return "", fmt.Errorf("转换聊天历史到 HTML 失败: %w", err)
	}
	return htmlFilePath, nil
}

// FlushSessionMemory 退出前强制提取当前会话的剩余记忆
func FlushSessionMemory() {
	FlushSessionMemoryWithManager(nil)
}

// FlushSessionMemoryWithManager 退出前强制提取当前会话的剩余记忆。
func FlushSessionMemoryWithManager(manager appstate.MemoryManager) {
	if manager == nil {
		return
	}
	recorder := getCliRecorder()
	manager.FlushFromRecorder(recorder, activeSessionID)
}

// SaveCLISessionHistory 保存 CLI 模式的可恢复会话历史。
func SaveCLISessionHistory() bool {
	recorder := getCliRecorder()
	if recorder.GetMessageCount() == 0 {
		return false
	}
	historyFile := filepath.Join(CLIHistoryDir, activeSessionID, eventlog.HistoryFileName)

	if err := recorder.SaveToFile(historyFile); err != nil {
		pterm.Error.Printfln("保存聊天历史失败: %v", err)
		return false
	}
	saveCliSessionMetadata(activeSessionID, cliSessionTitle)
	return true
}

// saveCliSessionMetadata 保存 CLI 会话元数据
// 如果提供了 userInput 且当前标题是默认时间戳格式，则更新为用户输入
func saveCliSessionMetadata(sessionID, userInput string) {
	sessionDir := filepath.Join(CLIHistoryDir, sessionID)
	now := time.Now()
	meta, err := eventlog.LoadMetadata(sessionDir)
	if err != nil {
		title := "未命名会话"
		if userInput != "" {
			title = truncateTitle(userInput)
		}
		meta = &eventlog.SessionMetadata{
			ID:        sessionID,
			Title:     title,
			Status:    "active",
			CreatedAt: now,
			UpdatedAt: now,
		}
	} else {
		meta.UpdatedAt = now
		if userInput != "" && isDefaultTitle(meta.Title) {
			meta.Title = truncateTitle(userInput)
		}
	}
	if err := eventlog.SaveMetadata(sessionDir, meta); err != nil {
		log.Printf("failed to save CLI session metadata: %v", err)
	}
}

// isDefaultTitle 检查标题是否为默认标题
func isDefaultTitle(title string) bool {
	if title == "未命名会话" {
		return true
	}
	_, err := time.Parse("2006-01-02 15:04:05", title)
	return err == nil
}

// truncateTitle 截断标题，最多 50 个字符（对中文安全）
func truncateTitle(s string) string {
	const maxLen = 50
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

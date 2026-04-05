// Package cli 提供 CLI 交互模式的会话管理、查询执行和信号处理
package cli

import (
	"context"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/common"
	"fkteams/engine"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/report"
	"fkteams/tools/approval"
	"fkteams/tools/ask"
	"fkteams/tui"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/adk"
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
	state           *QueryState
	runner          *adk.Runner
	autoReject      bool
	approveStores   []string // 自动批准的 store 列表
	callbackBuilder func(*fkevent.HistoryRecorder) func(fkevent.Event) error
}

// NewQueryExecutor 创建查询执行器
func NewQueryExecutor(runner *adk.Runner, state *QueryState) *QueryExecutor {
	return &QueryExecutor{
		state:           state,
		runner:          runner,
		callbackBuilder: fkevent.CLIEventCallback,
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
func (e *QueryExecutor) SetRunner(runner *adk.Runner) {
	e.runner = runner
}

// SetCallbackBuilder 设置事件回调构造器
func (e *QueryExecutor) SetCallbackBuilder(cb func(*fkevent.HistoryRecorder) func(fkevent.Event) error) {
	e.callbackBuilder = cb
}

// CLI 模式会话常量
const (
	CLISessionID = "cli"
)

// CLIHistoryDir CLI 会话历史存储目录
var CLIHistoryDir = common.SessionsDir()

// activeSessionID 当前活跃的会话 ID，每次启动时生成新 ID
var activeSessionID = CLISessionID

// cliSessionTitle 缓存第一次用户输入作为会话标题，仅在保存时使用
var cliSessionTitle string

// resumeSessionID 恢复会话的 ID，由 -r 参数设置
var resumeSessionID string

// NewDirectSessionID 生成基于 UUID 的唯一会话 ID
func NewDirectSessionID() string {
	return common.GenerateSessionID()
}

// SetResumeSessionID 设置要恢复的会话 ID
func SetResumeSessionID(sessionID string) {
	resumeSessionID = sessionID
}

// getCliRecorder 获取 CLI 模式的历史记录器
func getCliRecorder() *fkevent.HistoryRecorder {
	return fkevent.GlobalSessionManager.GetOrCreate(activeSessionID, CLIHistoryDir)
}

// BuildInputMessages 构建输入消息列表（包含历史对话，支持上下文压缩摘要）
func BuildInputMessages(input string) []adk.Message {
	recorder := getCliRecorder()
	return chatutil.BuildInputMessages(recorder, input)
}

// Execute 执行查询
func (e *QueryExecutor) Execute(ctx context.Context, input string) error {
	inputMessages := BuildInputMessages(input)
	recorder := getCliRecorder()
	countBeforeRun := recorder.GetMessageCount()
	recorder.RecordUserInput(input)

	// 缓存第一次输入作为会话标题（不立即创建文件，等用户保存时才写入）
	if cliSessionTitle == "" {
		cliSessionTitle = truncateTitle(input)
	}

	// 创建可取消的 context
	queryCtx, cancelFunc := context.WithCancel(ctx)

	// 显示加载动画，通过包装回调在首个事件到达时停止
	fmt.Println()
	spinner, _ := pterm.DefaultSpinner.Start("思考中...")
	stopSpinner := sync.OnceFunc(func() { spinner.Stop() })

	innerCallback := e.callbackBuilder(recorder)
	queryCtx = fkevent.WithCallback(queryCtx, func(event fkevent.Event) error {
		stopSpinner()
		return innerCallback(event)
	})

	// 注入统一审批注册表
	storeConfigs := []approval.StoreConfig{
		{Name: approval.StoreCommand},
		{Name: approval.StoreFile, Matcher: approval.DirMatchFunc},
		{Name: approval.StoreDispatch},
	}
	if len(e.approveStores) > 0 {
		queryCtx = approval.WithRegistry(queryCtx, approval.NewSelectiveRegistry(e.approveStores, storeConfigs...))
	} else {
		queryCtx = approval.WithRegistry(queryCtx, approval.NewRegistry(storeConfigs...))
	}

	// 设置摘要持久化回调
	queryCtx = summary.WithSummaryPersistCallback(queryCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})

	e.state.SetCancelFunc(cancelFunc)
	e.state.StartQuery()

	defer func() {
		stopSpinner()
		e.state.EndQuery()

		// 异步提取记忆
		if g.MemoryManager != nil {
			g.MemoryManager.ExtractFromRecorder(recorder, activeSessionID)
		}
	}()

	// 构建中断处理器
	var handler engine.InterruptHandler
	if e.autoReject {
		handler = engine.AutoRejectHandler()
	} else {
		handler = engine.CompositeCallbackHandler(e.promptApproval, e.promptAskQuestions)
	}

	startTime := time.Now()
	_, err := engine.New(e.runner, "fkteams").Run(queryCtx, inputMessages, engine.WithInterruptHandler(handler))
	if err != nil {
		if queryCtx.Err() != nil {
			pterm.Warning.Println("查询已中断")
			return nil
		}
		log.Printf("执行出错: %v", err)
		return nil
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	fmt.Printf("\n\033[1;32m✓ 完成\033[0m \033[90m(%s)\033[0m\n", elapsed)
	return nil
}

// promptApproval 提示用户审批，返回审批决定
func (e *QueryExecutor) promptApproval() int {
	if e.autoReject {
		pterm.Warning.Println("非交互模式，自动拒绝危险命令")
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
	fmt.Printf("\n\n")
	pterm.Info.Println("正在中断查询...")
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
	if g.MemoryManager == nil {
		return
	}
	recorder := getCliRecorder()
	g.MemoryManager.FlushFromRecorder(recorder, activeSessionID)
}

// AutoSaveCLIHistory 自动保存 CLI 模式的聊天历史（由 --save 参数控制）
func AutoSaveCLIHistory() {
	recorder := getCliRecorder()
	historyFile := filepath.Join(CLIHistoryDir, activeSessionID, "history.json")

	pterm.Info.Println("正在自动保存聊天历史...")
	if err := recorder.SaveToFile(historyFile); err != nil {
		pterm.Error.Printfln("保存聊天历史失败: %v", err)
	} else {
		pterm.Success.Printfln("成功保存聊天历史: %s", historyFile)
		saveCliSessionMetadata(activeSessionID, cliSessionTitle)
	}

	htmlFilePath, err := SaveChatHistoryToHTML()
	if err != nil {
		pterm.Error.Printfln("%v", err)
	} else {
		pterm.Success.Printfln("成功保存聊天历史到网页文件: %s", htmlFilePath)
	}
}

// saveCliSessionMetadata 保存 CLI 会话元数据
// 如果提供了 userInput 且当前标题是默认时间戳格式，则更新为用户输入
func saveCliSessionMetadata(sessionID, userInput string) {
	sessionDir := filepath.Join(CLIHistoryDir, sessionID)
	now := time.Now()
	meta, err := fkevent.LoadMetadata(sessionDir)
	if err != nil {
		title := "未命名会话"
		if userInput != "" {
			title = truncateTitle(userInput)
		}
		meta = &fkevent.SessionMetadata{
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
	if err := fkevent.SaveMetadata(sessionDir, meta); err != nil {
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

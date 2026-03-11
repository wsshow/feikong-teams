// Package cli 提供 CLI 交互模式的会话管理、查询执行和信号处理
package cli

import (
	"context"
	"fkteams/agents/middlewares/summary"
	"fkteams/chatutil"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/report"
	"fmt"
	"log"
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
	CLISessionID  = "cli"
	CLIHistoryDir = "./history/chat_history/"
)

// activeSessionID 当前活跃的会话 ID，每次启动时生成新 ID
var activeSessionID = CLISessionID

// resumeSessionID 恢复会话的 ID，由 -r 参数设置
var resumeSessionID string

// NewDirectSessionID 生成基于时间戳的唯一会话 ID
func NewDirectSessionID() string {
	return time.Now().Format("20060102_150405")
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

	// 创建可取消的 context，并设置 CLI 事件回调
	queryCtx, cancelFunc := context.WithCancel(ctx)
	queryCtx = fkevent.WithCallback(queryCtx, e.callbackBuilder(recorder))

	// 设置摘要持久化回调
	queryCtx = summary.WithSummaryPersistCallback(queryCtx, func(summaryText string) {
		recorder.SetSummary(summaryText, countBeforeRun)
	})

	e.state.SetCancelFunc(cancelFunc)
	e.state.StartQuery()

	defer func() {
		e.state.EndQuery()

		// 异步提取记忆
		if g.MemManager != nil {
			g.MemManager.ExtractFromRecorder(recorder, activeSessionID)
		}
	}()

	iter := e.runner.Run(queryCtx, inputMessages, adk.WithCheckPointID("fkteams"))

	// 显示加载动画
	fmt.Println()
	spinner, _ := pterm.DefaultSpinner.Start("思考中...")

	// 使用 channel 接收事件
	eventChan := make(chan struct {
		event *adk.AgentEvent
		ok    bool
	}, 1)

	go func() {
		for {
			event, ok := iter.Next()
			eventChan <- struct {
				event *adk.AgentEvent
				ok    bool
			}{event, ok}
			if !ok {
				close(eventChan)
				return
			}
		}
	}()

	startTime := time.Now()
	for {
		select {
		case <-queryCtx.Done():
			spinner.Stop()
			pterm.Warning.Println("查询已中断")
			return nil
		case result, ok := <-eventChan:
			spinner.Stop()
			// 二次检测 context 取消：eventChan 和 ctx.Done 可能同时就绪，
			// 需要确认收到事件后 context 未被取消，避免处理已过期的事件
			select {
			case <-queryCtx.Done():
				pterm.Warning.Println("查询已中断")
				return nil
			default:
			}

			if !ok {
				elapsed := time.Since(startTime).Round(time.Millisecond)
				fmt.Printf("\n\033[1;32m✓ 完成\033[0m \033[90m(%s)\033[0m\n", elapsed)
				return nil
			}
			if !result.ok {
				elapsed := time.Since(startTime).Round(time.Millisecond)
				fmt.Printf("\n\033[1;32m✓ 完成\033[0m \033[90m(%s)\033[0m\n", elapsed)
				return nil
			}
			if err := fkevent.ProcessAgentEvent(queryCtx, result.event); err != nil {
				log.Printf("Error processing event: %v", err)
				return err
			}
		}
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
	if g.MemManager == nil {
		return
	}
	recorder := getCliRecorder()
	g.MemManager.FlushFromRecorder(recorder, activeSessionID)
}

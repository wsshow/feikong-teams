package cli

import (
	"context"
	"fkteams/fkevent"
	"fkteams/report"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
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
	state  *QueryState
	runner *adk.Runner
}

// NewQueryExecutor 创建查询执行器
func NewQueryExecutor(runner *adk.Runner, state *QueryState) *QueryExecutor {
	return &QueryExecutor{
		state:  state,
		runner: runner,
	}
}

// SetRunner 设置 runner
func (e *QueryExecutor) SetRunner(runner *adk.Runner) {
	e.runner = runner
}

// CLI 模式会话常量
const (
	CLISessionID  = "cli"
	CLIHistoryDir = "./history/chat_history/"
)

// getCliRecorder 获取 CLI 模式的历史记录器
func getCliRecorder() *fkevent.HistoryRecorder {
	return fkevent.GlobalSessionManager.GetOrCreate(CLISessionID, CLIHistoryDir)
}

// BuildInputMessages 构建输入消息列表（包含历史对话）
func BuildInputMessages(input string) []adk.Message {
	var inputMessages []adk.Message
	recorder := getCliRecorder()
	agentMessages := recorder.GetMessages()
	if len(agentMessages) > 0 {
		var historyMessage strings.Builder
		for _, agentMessage := range agentMessages {
			fmt.Fprintf(&historyMessage, "%s: %s\n", agentMessage.AgentName, agentMessage.GetTextContent())
		}
		inputMessages = append(inputMessages, schema.SystemMessage(fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String())))
	}
	inputMessages = append(inputMessages, schema.UserMessage(input))
	return inputMessages
}

// Execute 执行查询
func (e *QueryExecutor) Execute(ctx context.Context, input string, useKeyboardMonitor bool, onInterrupt func()) error {
	inputMessages := BuildInputMessages(input)
	recorder := getCliRecorder()
	recorder.RecordUserInput(input)

	// 创建可取消的 context，并设置 CLI 事件回调
	queryCtx, cancelFunc := context.WithCancel(ctx)
	queryCtx = fkevent.WithCallback(queryCtx, fkevent.CLIEventCallback(recorder))
	e.state.SetCancelFunc(cancelFunc)
	e.state.StartQuery()

	// 启动键盘监听
	var stopKeyboardMonitor func()
	if useKeyboardMonitor {
		stopKeyboardMonitor = StartKeyboardMonitor(e.state)
	}
	defer func() {
		if stopKeyboardMonitor != nil {
			stopKeyboardMonitor()
		}
		e.state.EndQuery()
	}()

	iter := e.runner.Run(queryCtx, inputMessages, adk.WithCheckPointID("fkteams"))

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

	for {
		select {
		case <-queryCtx.Done():
			pterm.Warning.Println("查询已中断")
			if onInterrupt != nil {
				onInterrupt()
			}
			return nil
		case result, ok := <-eventChan:
			select {
			case <-queryCtx.Done():
				pterm.Warning.Println("查询已中断")
				if onInterrupt != nil {
					onInterrupt()
				}
				return nil
			default:
			}

			if !ok {
				return nil
			}
			if !result.ok {
				return nil
			}
			if err := fkevent.ProcessAgentEvent(queryCtx, result.event); err != nil {
				log.Printf("Error processing event: %v", err)
				return err
			}
		}
	}
}

// StartKeyboardMonitor 在查询期间监听 Ctrl+C
// 平台特定实现在 query_unix.go 和 query_windows.go 中

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

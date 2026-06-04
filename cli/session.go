package cli

import (
	"context"
	"fkteams/eventlog"
	"fkteams/fkevent"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/eino/adk"
	"github.com/pterm/pterm"
)

// ModeRunnerCreator 模式运行器创建回调
type ModeRunnerCreator func(ctx context.Context, mode WorkMode) (*adk.Runner, error)

// Session 交互会话，封装 CLI 交互的全部状态
type Session struct {
	InputHistory     []string
	CurrentMode      WorkMode
	ApproveStores    string // 自动批准的 store（逗号分隔: all/command/file/dispatch）
	queryState       *QueryState
	currentAgent     string
	createModeRunner ModeRunnerCreator
	callbackBuilder  func(*eventlog.HistoryRecorder) func(fkevent.Event) error
}

// NewSession 创建交互会话
func NewSession(mode WorkMode, inputHistory []string, createModeRunner ModeRunnerCreator) *Session {
	return &Session{
		InputHistory:     inputHistory,
		CurrentMode:      mode,
		queryState:       NewQueryState(),
		createModeRunner: createModeRunner,
	}
}

// GetQueryState 获取查询状态
func (s *Session) GetQueryState() *QueryState {
	return s.queryState
}

// StartSignalHandler 监听系统信号（SIGINT 运行时取消查询；其他信号转发为退出信号）
func (s *Session) StartSignalHandler(exitSignals chan os.Signal) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGINT, os.Interrupt:
				if s.queryState.IsRunning() {
					HandleCtrlC(s.queryState)
				}
			default:
				select {
				case exitSignals <- sig:
				default:
				}
			}
		}
	}()
}

// HandleDirect 非交互模式：执行单次查询后退出
func (s *Session) HandleDirect(ctx context.Context, r *adk.Runner, exitSignals chan os.Signal, query string) {
	s.InputHistory = append(s.InputHistory, query)

	if resumeSessionID != "" {
		activeSessionID = resumeSessionID
		pterm.Info.Printf("[非交互模式] 恢复会话: %s\n", activeSessionID)
	} else {
		activeSessionID = NewDirectSessionID()
		pterm.Info.Printf("[非交互模式] 会话 ID: %s\n", activeSessionID)
	}

	// 回显用户输入
	fmt.Printf("\n\033[1;90m╭─ [用户]\033[0m\n")
	fmt.Printf("\033[1;90m╰─▶ %s\033[0m\n", query)

	executor := NewQueryExecutor(r, s.queryState)
	executor.SetAutoReject(true)
	executor.SetApproveStores(s.ApproveStores)
	if s.callbackBuilder != nil {
		executor.SetCallbackBuilder(s.callbackBuilder)
	}
	if err := executor.Execute(ctx, query); err != nil {
		log.Printf("执行查询失败: %v", err)
	}

	select {
	case exitSignals <- syscall.SIGTERM:
	default:
	}
}

// HandleInteractive 交互模式：启动 REPL 循环
func (s *Session) HandleInteractive(ctx context.Context, r *adk.Runner, exitSignals chan os.Signal) {
	if resumeSessionID != "" {
		activeSessionID = resumeSessionID
	} else {
		activeSessionID = NewDirectSessionID()
	}

	defer func() {
		if r := recover(); r != nil {
			pterm.Error.Printfln("交互循环异常: %v", r)
			select {
			case exitSignals <- syscall.SIGTERM:
			default:
			}
		}
	}()

	rt := NewRuntime(ctx, s, r, exitSignals)
	if err := rt.Run(); err != nil {
		log.Printf("TUI runtime failed: %v", err)
		select {
		case exitSignals <- syscall.SIGTERM:
		default:
		}
	}
}

// SetCurrentAgent 设置当前智能体名称（用于 agent 命令初始化）
func (s *Session) SetCurrentAgent(name string) {
	s.currentAgent = name
}

// SetCallbackBuilder 设置事件回调构造器（用于自定义输出格式）
func (s *Session) SetCallbackBuilder(cb func(*eventlog.HistoryRecorder) func(fkevent.Event) error) {
	s.callbackBuilder = cb
}

// sessionModeSwitcher 模式切换器
type sessionModeSwitcher struct {
	session  *Session
	ctx      context.Context
	executor *QueryExecutor
}

// SwitchMode 切换工作模式
// 如果当前处于 @agent 单智能体模式，恢复到切换前的工作模式；否则循环切换（team → deep → group → custom → team）
func (m *sessionModeSwitcher) SwitchMode() (string, error) {
	var newMode WorkMode

	if m.session.currentAgent != "" {
		// 从 @agent 模式恢复到原工作模式
		newMode = m.session.CurrentMode
	} else {
		switch m.session.CurrentMode {
		case ModeTeam:
			newMode = ModeDeep
		case ModeDeep:
			newMode = ModeGroup
		case ModeGroup:
			newMode = ModeCustom
		case ModeCustom:
			newMode = ModeTeam
		default:
			newMode = ModeTeam
		}
	}

	newRunner, err := m.session.createModeRunner(m.ctx, newMode)
	if err != nil {
		return "", fmt.Errorf("failed to create runner for mode %s: %w", newMode, err)
	}
	if newRunner == nil {
		return "", fmt.Errorf("failed to create runner for mode: %s", newMode)
	}

	m.session.CurrentMode = newMode
	m.session.currentAgent = ""
	m.executor.SetRunner(newRunner)
	return newMode.String(), nil
}

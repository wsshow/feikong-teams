package runtime

import (
	"context"
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/appstate"
	appschedule "fkteams/internal/app/schedule"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/events"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pterm/pterm"
)

// ModeRunnerCreator 模式运行器创建回调
type ModeRunnerCreator func(ctx context.Context, mode WorkMode) (runtimeport.Runner, error)

// Session 交互会话，封装 CLI 交互的全部状态
type Session struct {
	InputHistory     []string
	CurrentMode      WorkMode
	ApproveStores    string // 自动批准的 store（逗号分隔: all/command/file/dispatch）
	queryState       *QueryState
	currentAgent     string
	createModeRunner ModeRunnerCreator
	callbackBuilder  func(*eventlog.HistoryRecorder) func(events.Event) error
	memory           appstate.MemoryManager
	scheduler        *appschedule.Service
	historyDir       string
	historyManager   *eventlog.SessionHistoryManager
	activeSessionID  string
	sessionTitle     string
	resumeSessionID  string
	temporary        bool
}

// NewSession 创建交互会话
func NewSession(mode WorkMode, inputHistory []string, createModeRunner ModeRunnerCreator) *Session {
	return &Session{
		InputHistory:     inputHistory,
		CurrentMode:      mode,
		queryState:       NewQueryState(),
		createModeRunner: createModeRunner,
		historyDir:       appdata.SessionsDir(),
		historyManager:   eventlog.NewSessionHistoryManager(),
		activeSessionID:  CLISessionID,
	}
}

// GetQueryState 获取查询状态
func (s *Session) GetQueryState() *QueryState {
	return s.queryState
}

// SetMemoryManager 设置会话使用的长期记忆管理器。
func (s *Session) SetMemoryManager(manager appstate.MemoryManager) {
	s.memory = manager
}

// SetScheduleService 设置会话使用的调度服务。
func (s *Session) SetScheduleService(service *appschedule.Service) {
	s.scheduler = service
}

// SetResumeSessionID 设置当前 CLI 会话要恢复的会话 ID。
func (s *Session) SetResumeSessionID(sessionID string) {
	s.resumeSessionID = sessionID
}

// SetTemporary 设置当前 CLI 会话是否不持久化。
func (s *Session) SetTemporary(v bool) {
	s.temporary = v
}

func (s *Session) isTemporary() bool {
	return s != nil && s.temporary
}

func (s *Session) sessionID() string {
	if s == nil || s.activeSessionID == "" {
		return CLISessionID
	}
	return s.activeSessionID
}

func (s *Session) recorder() *eventlog.HistoryRecorder {
	if s.historyManager == nil {
		s.historyManager = eventlog.NewSessionHistoryManager()
	}
	if s.historyDir == "" {
		s.historyDir = appdata.SessionsDir()
	}
	return s.historyManager.GetOrCreate(s.sessionID(), s.historyDir)
}

func (s *Session) setTitleFromInput(input string) {
	if s != nil && s.sessionTitle == "" {
		s.sessionTitle = truncateTitle(input)
	}
}

func (s *Session) activateSession(nonInteractive bool) {
	if s.resumeSessionID != "" {
		s.activeSessionID = s.resumeSessionID
		if nonInteractive {
			pterm.Info.Printf("[非交互模式] 恢复会话: %s\n", s.activeSessionID)
		}
		return
	}
	s.activeSessionID = NewDirectSessionID()
	if nonInteractive {
		if s.isTemporary() {
			pterm.Info.Printf("[非交互模式] 临时会话 ID: %s\n", s.activeSessionID)
		} else {
			pterm.Info.Printf("[非交互模式] 会话 ID: %s\n", s.activeSessionID)
		}
	}
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
func (s *Session) HandleDirect(ctx context.Context, r runtimeport.Runner, exitSignals chan os.Signal, query string) {
	s.InputHistory = append(s.InputHistory, query)

	s.activateSession(true)

	// 回显用户输入
	fmt.Printf("\n\033[1;90m╭─ [用户]\033[0m\n")
	fmt.Printf("\033[1;90m╰─▶ %s\033[0m\n", query)

	executor := NewQueryExecutor(r, s.queryState)
	executor.SetSession(s)
	executor.SetMemoryManager(s.memory)
	executor.SetScheduleService(s.scheduler)
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
func (s *Session) HandleInteractive(ctx context.Context, r runtimeport.Runner, exitSignals chan os.Signal) {
	s.activateSession(false)

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
func (s *Session) SetCallbackBuilder(cb func(*eventlog.HistoryRecorder) func(events.Event) error) {
	s.callbackBuilder = cb
}

// PrintResumeHint 打印当前实例会话的恢复命令。
func (s *Session) PrintResumeHint() {
	printResumeHint(s.sessionID())
}

func printResumeHint(sessionID string) {
	command := resumeCommand(sessionID)
	if command == "" {
		return
	}
	fmt.Fprintf(os.Stdout, "\n\033[90mResume this session with:\n%s\033[0m\n", command)
}

func resumeCommand(sessionID string) string {
	if sessionID == "" || sessionID == CLISessionID {
		return ""
	}
	return fmt.Sprintf("fkteams --resume %s", sessionID)
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

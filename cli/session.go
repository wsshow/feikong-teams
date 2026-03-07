package cli

import (
	"context"
	"errors"
	"fkteams/agents"
	"fkteams/runner"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/eino/adk"
	"github.com/pterm/pterm"
)

// ModeRunnerCreator 模式运行器创建回调
type ModeRunnerCreator func(ctx context.Context, mode WorkMode) *adk.Runner

// Session 交互会话，封装 CLI 交互的全部状态
type Session struct {
	InputHistory     []string
	inputBuffer      *InputBuffer
	CurrentMode      WorkMode
	queryState       *QueryState
	currentAgent     string
	createModeRunner ModeRunnerCreator
}

// NewSession 创建交互会话
func NewSession(mode WorkMode, inputHistory []string, createModeRunner ModeRunnerCreator) *Session {
	return &Session{
		InputHistory:     inputHistory,
		inputBuffer:      NewInputBuffer(),
		CurrentMode:      mode,
		queryState:       NewQueryState(),
		createModeRunner: createModeRunner,
	}
}

// GetQueryState 获取查询状态
func (s *Session) GetQueryState() *QueryState {
	return s.queryState
}

func (s *Session) handleInput(in string) string {
	cmd, needContinue := s.inputBuffer.HandleInput(in)
	if needContinue {
		return ""
	}
	return cmd
}

// currentPrefix 返回当前提示符前缀
func (s *Session) currentPrefix() string {
	if s.inputBuffer.IsContinuing() {
		return "请继续输入: "
	}
	if s.currentAgent != "" {
		return fmt.Sprintf("%s> ", s.currentAgent)
	}
	return s.CurrentMode.GetPromptPrefix()
}

// StartSignalHandler 监听系统信号（SIGINT 在查询运行时中断查询，否则忽略；其他信号转发为退出信号）
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
				// huh 在 raw 模式下已捕获 Ctrl+C，此处只处理查询期间的信号
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

	recorder := getCliRecorder()
	historyFile := CLIHistoryDir + "fkteams_chat_history_" + activeSessionID

	executor := NewQueryExecutor(r, s.queryState)
	if err := executor.Execute(ctx, query); err != nil {
		log.Printf("执行查询失败: %v", err)
	}

	fmt.Println()
	pterm.Info.Printf("[非交互模式] 任务完成，正在自动保存聊天历史...\n")
	if err := recorder.SaveToFile(historyFile); err != nil {
		pterm.Error.Printfln("[非交互模式] 保存聊天历史失败: %v", err)
	} else {
		pterm.Success.Printfln("[非交互模式] 成功保存聊天历史: %s", historyFile)
	}

	htmlFilePath, err := SaveChatHistoryToHTML()
	if err != nil {
		pterm.Error.Printfln("[非交互模式] %v", err)
	} else {
		pterm.Success.Printfln("[非交互模式] 成功保存聊天历史到网页文件: %s", htmlFilePath)
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
		pterm.Info.Printf("恢复会话: %s\n", activeSessionID)
	} else {
		activeSessionID = NewDirectSessionID()
		pterm.Info.Printf("会话 ID: %s\n", activeSessionID)
	}

	go func() {
		executor := NewQueryExecutor(r, s.queryState)
		modeSwitcher := &sessionModeSwitcher{session: s, ctx: ctx, executor: executor}
		cmdHandler := NewCommandHandler(modeSwitcher)

		suggestions := BuildSuggestions()
		suggestFn := func() []string { return suggestions }

		for {
			prefix := s.currentPrefix()

			// 续行模式不显示补全
			var fn func() []string
			if !s.inputBuffer.IsContinuing() {
				fn = suggestFn
			}

			in, err := ReadInput(prefix, fn)
			if err != nil {
				if errors.Is(err, ErrInterrupted) {
					s.inputBuffer.Reset()
					fmt.Println()
					continue
				}
				exitSignals <- syscall.SIGTERM
				return
			}

			input := s.handleInput(in)
			if s.inputBuffer.IsContinuing() {
				continue
			}

			// 检查是否切换智能体
			if agentName, query := ExtractAgentMention(input); agentName != "" {
				agentInfo := agents.GetAgentByName(agentName)
				if agentInfo == nil {
					pterm.Error.Printf("未找到智能体: %s\n", agentName)
					fmt.Println()
					continue
				}

				newAgent := agentInfo.Creator(ctx)
				newRunner := runner.CreateAgentRunner(ctx, newAgent)
				executor.SetRunner(newRunner)
				s.currentAgent = agentName
				pterm.Success.Printf("已切换到智能体: %s (%s)\n", agentName, agentInfo.Description)

				// 刷新补全候选（智能体切换后可能变化）
				suggestions = BuildSuggestions()

				if query != "" {
					s.InputHistory = append(s.InputHistory, query)
					if err := executor.Execute(ctx, query); err != nil {
						log.Printf("执行查询失败: %v", err)
					}
				}
				fmt.Printf("\n\n")
				continue
			}

			result := cmdHandler.Handle(input)
			switch result {
			case ResultExit:
				exitSignals <- syscall.SIGTERM
				return
			case ResultHandled:
				continue
			case ResultNotFound:
				// 不是命令，作为查询处理
			}

			s.InputHistory = append(s.InputHistory, input)

			if err := executor.Execute(ctx, input); err != nil {
				log.Printf("执行查询失败: %v", err)
			}
			fmt.Printf("\n\n")
		}
	}()
}

// sessionModeSwitcher 模式切换器
type sessionModeSwitcher struct {
	session  *Session
	ctx      context.Context
	executor *QueryExecutor
}

// SwitchMode 切换工作模式（循环切换：team → deep → group → custom → team）
func (m *sessionModeSwitcher) SwitchMode() (string, error) {
	var newMode WorkMode
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

	newRunner := m.session.createModeRunner(m.ctx, newMode)
	if newRunner == nil {
		return "", fmt.Errorf("failed to create runner for mode: %s", newMode)
	}

	m.session.CurrentMode = newMode
	m.session.currentAgent = ""
	m.executor.SetRunner(newRunner)
	return newMode.String(), nil
}

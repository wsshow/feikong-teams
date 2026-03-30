package cli

import (
	"context"
	"errors"
	"fkteams/agents"
	"fkteams/fkevent"
	"fkteams/runner"
	"fkteams/tui"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	callbackBuilder  func(*fkevent.HistoryRecorder) func(fkevent.Event) error
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

// currentPrefix 返回当前提示符前缀
func (s *Session) currentPrefix() string {
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
		pterm.Info.Printf("恢复会话: %s\n", activeSessionID)
	} else {
		activeSessionID = NewDirectSessionID()
		pterm.Info.Printf("会话 ID: %s\n", activeSessionID)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				pterm.Error.Printfln("交互循环异常: %v", r)
				select {
				case exitSignals <- syscall.SIGTERM:
				default:
				}
			}
		}()

		executor := NewQueryExecutor(r, s.queryState)
		executor.SetApproveStores(s.ApproveStores)
		modeSwitcher := &sessionModeSwitcher{session: s, ctx: ctx, executor: executor}
		cmdHandler := NewCommandHandler(modeSwitcher)

		for {
			prefix := s.currentPrefix()

			// readInput 循环读取输入，处理 # 内联文件引用
			var in string
			var trigger string
			inputText := "" // 累积的输入文本（含 #path 引用）
		readInput:
			for {
				opts := &tui.ReadLineOpts{
					History:      s.InputHistory,
					InitialValue: inputText,
				}
				var err error
				in, trigger, err = tui.ReadLine(prefix, opts)
				if err != nil {
					if errors.Is(err, tui.ErrInterrupted) {
						fmt.Println()
						break readInput
					}
					exitSignals <- syscall.SIGTERM
					return
				}

				if trigger == "#" {
					// # 触发文件/目录选择，选中后插入到文本中继续输入
					filePath, selectErr := SelectFileOrDir(GetWorkspaceDir())
					if selectErr != nil {
						if !errors.Is(selectErr, tui.ErrInterrupted) {
							pterm.Error.Printf("选择文件失败: %v\n", selectErr)
						}
						// 选择取消，保留已输入的文本继续
						inputText = in
						continue readInput
					}
					// 将 #path 插入到文本中
					if in != "" {
						inputText = in + " #" + filePath + " "
					} else {
						inputText = "#" + filePath + " "
					}
					pterm.FgGray.Println("已引用: " + filePath)
					continue readInput
				}

				// 非 # 触发，跳出循环
				break readInput
			}

			// 如果是中断后 continue 的情况
			if trigger == "" && in == "" && inputText == "" {
				continue
			}

			// 触发字符立即响应（@/）
			switch trigger {
			case "@":
				agentName, selectErr := SelectAgent()
				if selectErr != nil {
					if !errors.Is(selectErr, tui.ErrInterrupted) {
						pterm.Error.Printf("选择智能体失败: %v\n", selectErr)
					}
					continue
				}
				s.switchAgent(ctx, executor, agentName)
				continue
			case "/":
				cmd, selectErr := SelectCommand()
				if selectErr != nil {
					if !errors.Is(selectErr, tui.ErrInterrupted) {
						pterm.Error.Printf("选择命令失败: %v\n", selectErr)
					}
					continue
				}
				result := cmdHandler.Handle(cmd)
				if result == ResultExit {
					exitSignals <- syscall.SIGTERM
					return
				}
				continue
			}

			// 如果输入以 \ 结尾，打开 textarea 多行编辑器
			input := strings.TrimSpace(in)
			if before, ok := strings.CutSuffix(input, "\\"); ok {
				multiText, multiErr := tui.ReadMultiLine(strings.TrimSpace(before))
				if multiErr != nil {
					if !errors.Is(multiErr, tui.ErrInterrupted) {
						pterm.Error.Printf("多行输入失败: %v\n", multiErr)
					}
					continue
				}
				input = strings.TrimSpace(multiText)
				if input == "" {
					continue
				}
			}

			// 回显用户输入
			if input != "" {
				fmt.Printf("\n\033[1;90m╭─ [用户]\033[0m\n")
				fmt.Printf("\033[1;90m╰─▶ %s\033[0m\n", input)
			}

			// 检查是否切换智能体（@智能体名 查询内容）
			if agentName, query := ExtractAgentMention(input); agentName != "" {
				s.switchAgent(ctx, executor, agentName)
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

// SetCurrentAgent 设置当前智能体名称（用于 agent 命令初始化）
func (s *Session) SetCurrentAgent(name string) {
	s.currentAgent = name
}

// SetCallbackBuilder 设置事件回调构造器（用于自定义输出格式）
func (s *Session) SetCallbackBuilder(cb func(*fkevent.HistoryRecorder) func(fkevent.Event) error) {
	s.callbackBuilder = cb
}

// switchAgent 切换到指定智能体
func (s *Session) switchAgent(ctx context.Context, executor *QueryExecutor, agentName string) {
	agentInfo := agents.GetAgentByName(agentName)
	if agentInfo == nil {
		pterm.Error.Printf("未找到智能体: %s\n", agentName)
		fmt.Println()
		return
	}

	newAgent := agentInfo.Creator(ctx)
	newRunner := runner.CreateAgentRunner(ctx, newAgent)
	executor.SetRunner(newRunner)
	s.currentAgent = agentName
	pterm.Success.Printf("已切换到智能体: %s (%s)\n", agentName, agentInfo.Description)
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

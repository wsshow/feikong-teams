package cli

import (
	"context"
	"errors"
	"fkteams/agents"
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
		modeSwitcher := &sessionModeSwitcher{session: s, ctx: ctx, executor: executor}
		cmdHandler := NewCommandHandler(modeSwitcher)

		for {
			prefix := s.currentPrefix()

			if s.inputBuffer.IsContinuing() {
				pterm.FgGray.Println("▼ " + s.inputBuffer.AccumulatedText())
			}

			in, trigger, err := tui.ReadLine(prefix, s.InputHistory)
			if err != nil {
				if errors.Is(err, tui.ErrInterrupted) {
					s.inputBuffer.Reset()
					fmt.Println()
					continue
				}
				exitSignals <- syscall.SIGTERM
				return
			}

			// 回显用户输入
			if trigger == "" && in != "" {
				pterm.FgGray.Printfln("%s%s", prefix, in)
			}

			// 触发字符立即响应（@/#/）
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
			case "#":
				var files []string
				for {
					filePath, selectErr := SelectFile(GetWorkspaceDir())
					if selectErr != nil {
						if !errors.Is(selectErr, tui.ErrInterrupted) {
							pterm.Error.Printf("选择文件失败: %v\n", selectErr)
						}
						break
					}
					files = append(files, "#"+filePath)
					pterm.FgGray.Println("已选择: " + strings.Join(files, " "))

					query, nextTrigger, queryErr := tui.ReadLine("输入查询 (# 继续添加文件 | 回车发送): ")
					if queryErr != nil {
						if !errors.Is(queryErr, tui.ErrInterrupted) {
							pterm.Error.Printf("读取查询失败: %v\n", queryErr)
						}
						files = nil
						break
					}
					if nextTrigger == "#" {
						continue
					}
					// 如果触发了 @ 或 /，放弃已选文件，直接处理触发
					if nextTrigger == "@" || nextTrigger == "/" {
						pterm.FgGray.Println("已取消文件引用")
						switch nextTrigger {
						case "@":
							agentName, selectErr := SelectAgent()
							if selectErr != nil {
								if !errors.Is(selectErr, tui.ErrInterrupted) {
									pterm.Error.Printf("选择智能体失败: %v\n", selectErr)
								}
							} else {
								s.switchAgent(ctx, executor, agentName)
							}
						case "/":
							cmd, selectErr := SelectCommand()
							if selectErr != nil {
								if !errors.Is(selectErr, tui.ErrInterrupted) {
									pterm.Error.Printf("选择命令失败: %v\n", selectErr)
								}
							} else {
								result := cmdHandler.Handle(cmd)
								if result == ResultExit {
									exitSignals <- syscall.SIGTERM
									return
								}
							}
						}
						break
					}
					// 构建最终输入
					fileRefs := strings.Join(files, " ")
					var finalInput string
					if query == "" {
						finalInput = fileRefs
					} else {
						finalInput = query + " " + fileRefs
					}
					pterm.FgGray.Printfln("> %s", finalInput)
					s.InputHistory = append(s.InputHistory, finalInput)
					if err := executor.Execute(ctx, finalInput); err != nil {
						log.Printf("执行查询失败: %v", err)
					}
					fmt.Printf("\n\n")
					break
				}
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

			input := s.handleInput(in)
			if s.inputBuffer.IsContinuing() {
				continue
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

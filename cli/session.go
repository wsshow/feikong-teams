package cli

import (
	"context"
	"fkteams/agents"
	"fkteams/runner"
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/atotto/clipboard"
	"github.com/c-bata/go-prompt"
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

func (s *Session) changeLivePrefix() (string, bool) {
	if s.inputBuffer.IsContinuing() {
		return "请继续输入: ", true
	}
	prefix := s.CurrentMode.GetPromptPrefix()
	if s.currentAgent != "" {
		prefix = fmt.Sprintf("%s> ", s.currentAgent)
	}
	return prefix, true
}

// StartSignalHandler 监听系统信号
func (s *Session) StartSignalHandler(rawSignals chan os.Signal, exitSignals chan os.Signal) {
	go func() {
		for sig := range rawSignals {
			if sig == syscall.SIGINT {
				if s.queryState.IsRunning() {
					HandleCtrlC(s.queryState)
					continue
				}
			}
			select {
			case exitSignals <- sig:
			default:
			}
		}
	}()
}

// HandleDirect 非交互模式：执行单次查询后退出
func (s *Session) HandleDirect(ctx context.Context, r *adk.Runner, exitSignals chan os.Signal, query string) {
	s.InputHistory = append(s.InputHistory, query)

	recorder := getCliRecorder()
	historyFile := CLIHistoryDir + "fkteams_chat_history_" + CLISessionID
	if err := recorder.LoadFromFile(historyFile); err == nil {
		pterm.Success.Println("[非交互模式] 自动加载聊天历史")
	}

	executor := NewQueryExecutor(r, s.queryState)
	if err := executor.Execute(ctx, query, true, nil); err != nil {
		log.Printf("执行查询失败: %v", err)
	}

	fmt.Println()
	pterm.Info.Printf("[非交互模式] 任务完成，正在自动保存聊天历史...\n")
	if err := recorder.SaveToFile(historyFile); err != nil {
		pterm.Error.Printfln("[非交互模式] 保存聊天历史失败: %v", err)
	} else {
		pterm.Success.Println("[非交互模式] 成功保存聊天历史到默认文件")
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
	go func() {
		executor := NewQueryExecutor(r, s.queryState)

		modeSwitcher := &sessionModeSwitcher{session: s, ctx: ctx, executor: executor}
		cmdHandler := NewCommandHandler(modeSwitcher)

		pasteKeyBind := prompt.KeyBind{
			Key: prompt.ControlV,
			Fn: func(buf *prompt.Buffer) {
				text, _ := clipboard.ReadAll()
				buf.InsertText(fmt.Sprintf("%s\n", text), false, true)
			},
		}

		p := prompt.New(nil,
			Completer,
			prompt.OptionTitle("FeiKong Teams"),
			prompt.OptionPrefixTextColor(prompt.Cyan),
			prompt.OptionSuggestionTextColor(prompt.White),
			prompt.OptionSuggestionBGColor(prompt.Black),
			prompt.OptionDescriptionTextColor(prompt.White),
			prompt.OptionDescriptionBGColor(prompt.Black),
			prompt.OptionHistory(s.InputHistory),
			prompt.OptionAddKeyBind(pasteKeyBind),
			prompt.OptionLivePrefix(s.changeLivePrefix),
		)

		for {
			in := p.Input()
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

				if query != "" {
					s.InputHistory = append(s.InputHistory, query)
					if err := executor.Execute(ctx, query, true, nil); err != nil {
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

			if err := executor.Execute(ctx, input, true, nil); err != nil {
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

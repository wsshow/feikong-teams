package main

import (
	"context"
	"fkteams/cli"
	"fkteams/common"
	"fkteams/config"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/runner"
	"fkteams/server"
	"fkteams/update"
	"fkteams/version"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/atotto/clipboard"
	"github.com/c-bata/go-prompt"
	"github.com/cloudwego/eino/adk"
	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	"github.com/spf13/pflag"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

var (
	inputHistory  []string           // 输入历史记录
	inputBuffer   *cli.InputBuffer   // 输入缓冲区
	currentMode   cli.WorkMode       // 当前工作模式
	queryState    *cli.QueryState    // 查询状态管理
	queryExecutor *cli.QueryExecutor // 查询执行器
)

func init() {
	inputBuffer = cli.NewInputBuffer()
	queryState = cli.NewQueryState()
}

func handleInput(in string) (finalCmd string) {
	cmd, needContinue := inputBuffer.HandleInput(in)
	if needContinue {
		return ""
	}
	return cmd
}

func changeLivePrefix() (string, bool) {
	if inputBuffer.IsContinuing() {
		return "请继续输入: ", true
	}
	return currentMode.GetPromptPrefix(), true
}

func completer(d prompt.Document) []prompt.Suggest {
	if d.GetWordBeforeCursor() == "" {
		return []prompt.Suggest{}
	}
	s := []prompt.Suggest{
		{Text: "quit", Description: "退出"},
		{Text: "load_chat_history", Description: "加载聊天历史"},
		{Text: "save_chat_history", Description: "保存聊天历史"},
		{Text: "clear_chat_history", Description: "清空聊天历史"},
		{Text: "clear_todo", Description: "清空待办事项"},
		{Text: "switch_work_mode", Description: "切换工作模式(团队模式/多智能体讨论模式)"},
		{Text: "save_chat_history_to_html", Description: "保存完整聊天历史到 HTML 文件"},
		{Text: "save_chat_history_to_markdown", Description: "保存完整聊天历史到 Markdown 文件"},
		{Text: "help", Description: "帮助信息"},
	}
	if d.TextBeforeCursor() == "/" {
		return s
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

// startSignalHandler 监听系统信号
func startSignalHandler(rawSignals chan os.Signal, exitSignals chan os.Signal) {
	go func() {
		for sig := range rawSignals {
			select {
			case exitSignals <- sig:
				if sig == syscall.SIGINT {
					cli.HandleCtrlC(queryState)
				}
			default:
			}
		}
	}()
}

func handleDirect(ctx context.Context, r *adk.Runner, exitSignals chan os.Signal, query string) {
	inputHistory = append(inputHistory, query)

	// 自动加载聊天历史
	if err := fkevent.GlobalHistoryRecorder.LoadFromDefaultFile(); err == nil {
		pterm.Success.Println("[非交互模式] 自动加载聊天历史")
	}

	// 执行查询（非交互模式也需要键盘监听来检测 Ctrl+C）
	executor := cli.NewQueryExecutor(r, queryState)
	if err := executor.Execute(ctx, query, true, nil); err != nil {
		log.Printf("执行查询失败: %v", err)
	}

	fmt.Println()
	// 保存聊天历史
	pterm.Info.Printf("[非交互模式] 任务完成，正在自动保存聊天历史...\n")
	if err := fkevent.GlobalHistoryRecorder.SaveToDefaultFile(); err != nil {
		pterm.Error.Printfln("[非交互模式] 保存聊天历史失败: %v", err)
	} else {
		pterm.Success.Println("[非交互模式] 成功保存聊天历史到默认文件")
	}

	// 保存为 HTML 文件
	htmlFilePath, err := cli.SaveChatHistoryToHTML()
	if err != nil {
		pterm.Error.Printfln("[非交互模式] %v", err)
	} else {
		pterm.Success.Printfln("[非交互模式] 成功保存聊天历史到网页文件: %s", htmlFilePath)
	}
	// 非阻塞发送退出信号
	select {
	case exitSignals <- syscall.SIGTERM:
	default:
	}
}

func handleInteractive(ctx context.Context, r *adk.Runner, exitSignals chan os.Signal) {
	go func() {
		// 创建查询执行器
		executor := cli.NewQueryExecutor(r, queryState)

		// 创建模式切换器
		modeSwitcher := &interactiveModeSwitcher{ctx: ctx, executor: executor}

		// 创建命令处理器
		cmdHandler := cli.NewCommandHandler(modeSwitcher)

		pasteKeyBind := prompt.KeyBind{
			Key: prompt.ControlV,
			Fn: func(buf *prompt.Buffer) {
				text, _ := clipboard.ReadAll()
				buf.InsertText(fmt.Sprintf("%s\n", text), false, true)
			},
		}

		p := prompt.New(nil,
			completer,
			prompt.OptionTitle("FeiKong Teams"),
			prompt.OptionPrefixTextColor(prompt.Cyan),
			prompt.OptionSuggestionTextColor(prompt.White),
			prompt.OptionSuggestionBGColor(prompt.Black),
			prompt.OptionDescriptionTextColor(prompt.White),
			prompt.OptionDescriptionBGColor(prompt.Black),
			prompt.OptionHistory(inputHistory),
			prompt.OptionAddKeyBind(pasteKeyBind),
			prompt.OptionLivePrefix(changeLivePrefix),
		)

		for {
			in := p.Input()
			input := handleInput(in)
			if inputBuffer.IsContinuing() {
				continue
			}

			// 处理命令
			result := cmdHandler.Handle(input)
			switch result {
			case cli.ResultExit:
				exitSignals <- syscall.SIGTERM
				return
			case cli.ResultHandled:
				continue
			case cli.ResultNotFound:
				// 不是命令，作为查询处理
			}

			inputHistory = append(inputHistory, input)

			// 执行查询（交互模式需要键盘监听，因为 go-prompt 会拦截 SIGINT）
			if err := executor.Execute(ctx, input, true, nil); err != nil {
				log.Printf("执行查询失败: %v", err)
			}
			fmt.Println()
		}
	}()
}

// interactiveModeSwitcher 交互模式切换器
type interactiveModeSwitcher struct {
	ctx      context.Context
	executor *cli.QueryExecutor
}

// SwitchMode 切换工作模式
func (s *interactiveModeSwitcher) SwitchMode() (string, error) {
	var newRunner *adk.Runner
	switch currentMode {
	case cli.ModeTeam:
		newRunner = loopAgentMode(s.ctx)
		currentMode = cli.ModeGroup
	case cli.ModeGroup:
		newRunner = supervisorMode(s.ctx)
		currentMode = cli.ModeTeam
	default:
		return "", fmt.Errorf("未知的当前工作模式: %s", currentMode)
	}
	s.executor.SetRunner(newRunner)
	return currentMode.String(), nil
}

func main() {
	var (
		checkUpdates   bool
		checkVersion   bool
		generateEnv    bool
		generateConfig bool
		web            bool
		workMode       string
		query          string
	)
	pflag.BoolVarP(&checkUpdates, "update", "u", false, "检查更新并退出")
	pflag.BoolVarP(&checkVersion, "version", "v", false, "显示版本信息并退出")
	pflag.BoolVarP(&generateEnv, "generate-env", "g", false, "生成示例.env文件并退出")
	pflag.BoolVarP(&generateConfig, "generate-config", "c", false, "生成示例配置文件并退出")
	pflag.BoolVarP(&web, "web", "w", false, "启动Web服务器")
	pflag.StringVarP(&query, "query", "q", "", "直接查询模式，执行完查询后退出")
	pflag.StringVarP(&workMode, "work-mode", "m", "team", "工作模式: team 或 group 或 custom")
	pflag.Parse()

	if checkVersion {
		info := version.Get()
		fmt.Printf("fkteams: %s\n", info)
		return
	}

	if generateEnv {
		err := common.GenerateExampleEnv(".env.example")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("成功生成示例.env文件: .env.example")
		return
	}

	if generateConfig {
		err := config.GenerateExample()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("成功生成示例配置文件: config/config.toml")
		return
	}

	// 加载环境变量
	err := godotenv.Load()
	if err != nil {
		fmt.Println("加载 .env 文件失败，请确保已创建该文件")
		fmt.Println("可以使用 --generate-env 或者 -g 参数生成示例文件")
		return
	}

	if checkUpdates {
		err := update.SelfUpdate("wsshow", "feikong-teams")
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	if web {
		server.Run()
		return
	}

	var r *adk.Runner
	ctx, done := context.WithCancel(context.Background())

	currentMode = cli.ParseWorkMode(workMode)

	switch currentMode {
	case cli.ModeTeam:
		r = supervisorMode(ctx)
	case cli.ModeGroup:
		r = loopAgentMode(ctx)
	case cli.ModeCustom:
		r = customSupervisorMode(ctx)
	default:
		pterm.Error.Println("暂不支持该模式：", workMode)
		return
	}

	defer func() {
		err = g.Cleaner.ExecuteAndClear()
		if err != nil {
			log.Fatalf("清理资源失败: %v", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 退出信号 channel
	exitSignals := make(chan os.Signal, 1)

	// 启动信号处理 goroutine
	startSignalHandler(signals, exitSignals)

	inputHistoryPath := "./history/input_history/fkteams_input_history"
	inputHistory, err = common.LoadHistory(inputHistoryPath, 100)
	if err != nil {
		log.Fatalf("加载输入历史失败: %v", err)
	}

	if query != "" {
		handleDirect(ctx, r, exitSignals, query)
	} else {
		handleInteractive(ctx, r, exitSignals)
	}

	sig := <-exitSignals
	pterm.Info.Printfln("收到信号: %v, 开始退出前的清理...", sig)

	done()

	err = common.SaveHistory(inputHistoryPath, inputHistory)
	if err != nil {
		log.Fatalf("保存输入历史失败: %v", err)
	}

	pterm.Success.Println("成功退出")
}

func supervisorMode(ctx context.Context) *adk.Runner {
	fmt.Printf("欢迎来到非空小队: %s\n", version.Get())
	return runner.CreateSupervisorRunner(ctx)
}

func loopAgentMode(ctx context.Context) *adk.Runner {
	fmt.Printf("欢迎来到非空小队 - 多智能体讨论模式: %s\n", version.Get())
	runner.PrintLoopAgentsInfo(ctx)
	return runner.CreateLoopAgentRunner(ctx)
}

func customSupervisorMode(ctx context.Context) *adk.Runner {
	fmt.Printf("欢迎来到非空小队: %s\n", version.Get())
	runner.PrintCustomAgentsInfo(ctx)
	return runner.CreateCustomSupervisorRunner(ctx)
}

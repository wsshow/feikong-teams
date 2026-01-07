package main

import (
	"context"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/discussant"
	"fkteams/agents/leader"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/visitor"
	"fkteams/common"
	"fkteams/config"
	"fkteams/fkevent"
	"fkteams/report"
	"fkteams/update"
	"fkteams/version"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/atotto/clipboard"
	"github.com/c-bata/go-prompt"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/supervisor"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	"github.com/spf13/pflag"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

var (
	inputHistory    []string // 输入历史记录
	fullBuffer      []string // 存储已输入的所有行
	isContinuing    bool     // 是否处于续行状态
	currentWorkMode string   // 当前工作模式
)

func handleInput(in string) (finalCmd string) {
	cleanIn := strings.TrimSpace(in)
	// 如果以 \ 结尾，表示要续行
	if before, ok := strings.CutSuffix(cleanIn, "\\"); ok {
		fullBuffer = append(fullBuffer, before)
		isContinuing = true
		return
	}
	// 否则，合并所有行并执行
	fullBuffer = append(fullBuffer, cleanIn)
	finalCmd = strings.Join(fullBuffer, "\n")
	// 执行完毕，重置状态
	fullBuffer = []string{}
	isContinuing = false
	return finalCmd
}

func changeLivePrefix() (string, bool) {
	prefix := ""
	switch currentWorkMode {
	case "team":
		prefix = "团队模式> "
	case "group":
		prefix = "多智能体讨论模式> "
	default:
		prefix = "未知模式> "
	}
	if isContinuing {
		return "请继续输入: ", true
	}
	return prefix, true
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

func handleDirect(ctx context.Context, runner *adk.Runner, signals chan os.Signal, query string) {
	var inputMessages []adk.Message
	input := query
	inputHistory = append(inputHistory, input)
	inputMessages = []adk.Message{}
	err := fkevent.GlobalHistoryRecorder.LoadFromDefaultFile()
	if err == nil {
		pterm.Success.Println("[非交互模式] 自动加载聊天历史")
	}
	agentMessages := fkevent.GlobalHistoryRecorder.GetMessages()
	if len(agentMessages) > 0 {
		var historyMessage strings.Builder
		for _, agentMessage := range agentMessages {
			fmt.Fprintf(&historyMessage, "%s: %s\n", agentMessage.AgentName, agentMessage.Content)
		}
		inputMessages = append(inputMessages, schema.SystemMessage(fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String())))
	}
	inputMessages = append(inputMessages, schema.UserMessage(input))
	fkevent.GlobalHistoryRecorder.RecordUserInput(input)
	iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
			log.Printf("Error processing event: %v", err)
			break
		}
	}
	fmt.Printf("\n\n")
	// 保存聊天历史
	pterm.Info.Printf("[非交互模式] 任务完成，正在自动保存聊天历史...\n")
	err = fkevent.GlobalHistoryRecorder.SaveToDefaultFile()
	if err != nil {
		pterm.Error.Printfln("[非交互模式] 保存聊天历史失败: %v", err)
	} else {
		pterm.Success.Println("[非交互模式] 成功保存聊天历史到默认文件")
	}
	// 保存为 HTML 文件
	filePath, err := fkevent.GlobalHistoryRecorder.SaveToMarkdownWithTimestamp()
	if err != nil {
		pterm.Error.Printfln("[非交互模式] 保存聊天历史到 Markdown 失败: %v", err)
		signals <- syscall.SIGTERM
		return
	}
	htmlFilePath, err := report.ConvertMarkdownFileToNiceHTMLFile(filePath)
	if err != nil {
		pterm.Error.Printfln("[非交互模式] 转换聊天历史到网页失败: %v", err)
		signals <- syscall.SIGTERM
		return
	}
	pterm.Success.Printfln("[非交互模式] 成功保存聊天历史到网页文件: %s", htmlFilePath)
	signals <- syscall.SIGTERM
}

func handleInteractive(ctx context.Context, runner *adk.Runner, signals chan os.Signal) {
	go func() {
		var inputMessages []adk.Message

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
			if isContinuing {
				continue
			}

			if input == "q" || input == "quit" || input == "" {
				pterm.Info.Println("谢谢使用，再见！")
				signals <- syscall.SIGTERM
				return
			}
			if input == "help" {
				pterm.Println("帮助信息: 输入您的问题以获取回答，输入 'quit' 或 'q' 退出程序。")
				continue
			}
			if input == "load_chat_history" {
				err := fkevent.GlobalHistoryRecorder.LoadFromDefaultFile()
				if err != nil {
					pterm.Error.Printfln("加载聊天历史失败: %v", err)
				} else {
					pterm.Success.Println("成功加载聊天历史")
				}
				continue
			}
			if input == "save_chat_history" {
				err := fkevent.GlobalHistoryRecorder.SaveToDefaultFile()
				if err != nil {
					pterm.Error.Printfln("保存聊天历史失败: %v", err)
				} else {
					pterm.Success.Println("成功保存聊天历史")
				}
				continue
			}
			if input == "clear_chat_history" {
				fkevent.GlobalHistoryRecorder.Clear()
				pterm.Success.Println("成功清空当前聊天历史")
				continue
			}
			if input == "save_chat_history_to_markdown" {
				filePath, err := fkevent.GlobalHistoryRecorder.SaveToMarkdownWithTimestamp()
				if err != nil {
					pterm.Error.Printfln("保存聊天历史到 Markdown 失败: %v", err)
				} else {
					pterm.Success.Printfln("成功保存聊天历史到 Markdown 文件: %s", filePath)
				}
				continue
			}
			if input == "clear_todo" {
				err := leader.ClearTodoTool()
				if err != nil {
					pterm.Error.Printfln("清空待办事项失败: %v", err)
					continue
				}
				pterm.Success.Println("成功清空待办事项")
				continue
			}
			if input == "switch_work_mode" {
				switch currentWorkMode {
				case "team":
					runner = loopAgentMode(ctx)
					currentWorkMode = "group"
				case "group":
					runner = supervisorMode(ctx)
					currentWorkMode = "team"
				default:
					pterm.Error.Println("未知的当前工作模式: ", currentWorkMode)
					continue
				}
				pterm.Success.Printfln("成功切换到工作模式: %s", currentWorkMode)
				continue
			}
			if input == "save_chat_history_to_html" {
				filePath, err := fkevent.GlobalHistoryRecorder.SaveToMarkdownWithTimestamp()
				if err != nil {
					pterm.Error.Printfln("保存聊天历史到 Markdown 失败: %v", err)
					continue
				}
				htmlFilePath, err := report.ConvertMarkdownFileToNiceHTMLFile(filePath)
				if err != nil {
					pterm.Error.Printfln("转换聊天历史到 HTML 失败: %v", err)
					continue
				}
				pterm.Success.Printfln("成功保存聊天历史到 HTML 文件: %s", htmlFilePath)
				continue
			}

			inputHistory = append(inputHistory, input)
			// 构建消息列表（包含历史对话）
			inputMessages = []adk.Message{}
			agentMessages := fkevent.GlobalHistoryRecorder.GetMessages()
			if len(agentMessages) > 0 {
				var historyMessage strings.Builder
				for _, agentMessage := range agentMessages {
					fmt.Fprintf(&historyMessage, "%s: %s\n", agentMessage.AgentName, agentMessage.Content)
				}
				inputMessages = append(inputMessages, schema.SystemMessage(fmt.Sprintf("以下是之前的对话历史:\n---\n%s\n---\n", historyMessage.String())))
			}
			// 添加当前用户输入
			inputMessages = append(inputMessages, schema.UserMessage(input))
			// 记录用户输入到历史
			fkevent.GlobalHistoryRecorder.RecordUserInput(input)

			iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
			for {
				event, ok := iter.Next()
				if !ok {
					break
				}

				if err := fkevent.ProcessAgentEvent(ctx, event); err != nil {
					log.Printf("Error processing event: %v", err)
					break
				}
			}
			fmt.Println()
		}
	}()
}

func main() {
	var (
		checkUpdates   bool
		checkVersion   bool
		generateEnv    bool
		generateConfig bool
		workMode       string
		query          string
	)
	pflag.BoolVarP(&checkUpdates, "update", "u", false, "检查更新并退出")
	pflag.BoolVarP(&checkVersion, "version", "v", false, "显示版本信息并退出")
	pflag.BoolVarP(&generateEnv, "generate-env", "g", false, "生成示例.env文件并退出")
	pflag.BoolVarP(&generateConfig, "generate-config", "c", false, "生成示例配置文件并退出")
	pflag.StringVarP(&query, "query", "q", "", "直接查询模式，执行完查询后退出")
	pflag.StringVarP(&workMode, "work-mode", "m", "team", "工作模式: team 或 group")
	pflag.Parse()

	if checkVersion {
		info := version.Get()
		fmt.Printf("fkteams: %s\n", info)
		return
	}

	if checkUpdates {
		err := update.SelfUpdate("wsshow", "feikong-teams")
		if err != nil {
			log.Fatal(err)
		}
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

	var err error
	var runner *adk.Runner
	ctx, done := context.WithCancel(context.Background())

	currentWorkMode = workMode

	switch workMode {
	case "team":
		runner = supervisorMode(ctx)
	case "group":
		runner = loopAgentMode(ctx)
	default:
		pterm.Error.Println("暂不支持该模式：", workMode)
		return
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	inputHistoryPath := "./history/input_history/fkteams_input_history"
	inputHistory, err = common.LoadHistory(inputHistoryPath, 100)
	if err != nil {
		log.Fatalf("加载输入历史失败: %v", err)
	}

	if query != "" {
		handleDirect(ctx, runner, signals, query)
	} else {
		handleInteractive(ctx, runner, signals)
	}

	sig := <-signals
	pterm.Info.Printfln("收到信号: %v, 开始退出前的清理...", sig)

	done()

	err = common.SaveHistory(inputHistoryPath, inputHistory)
	if err != nil {
		log.Fatalf("保存输入历史失败: %v", err)
	}

	pterm.Success.Println("成功退出")
}

func supervisorMode(ctx context.Context) *adk.Runner {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	fmt.Printf("欢迎来到非空小队: %s\n", version.Get())

	leaderAgent := leader.NewAgent()
	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	subAgents := []adk.Agent{searcherAgent, storytellerAgent}

	if os.Getenv("FEIKONG_CODER_ENABLED") == "true" {
		coderAgent := coder.NewAgent()
		subAgents = append(subAgents, coderAgent)
	}

	if os.Getenv("FEIKONG_CMDER_ENABLED") == "true" {
		cmderAgent := cmder.NewAgent()
		subAgents = append(subAgents, cmderAgent)
	}

	if os.Getenv("FEIKONG_SSH_VISITOR_ENABLED") == "true" {
		visitorAgent := visitor.NewAgent()
		defer visitor.CloseSSHClient()
		subAgents = append(subAgents, visitorAgent)
	}

	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: leaderAgent,
		SubAgents:  subAgents,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           supervisorAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}

func loopAgentMode(ctx context.Context) *adk.Runner {
	teamConfig, err := config.Get()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("欢迎来到非空小队 - 多智能体讨论模式: %s\n", version.Get())

	var subAgents []adk.Agent
	for _, member := range teamConfig.Roundtable.Members {
		agent := discussant.NewAgent(member)
		subAgents = append(subAgents, agent)
	}

	fmt.Printf("本次讨论的成员有: ")
	var names []string
	for _, subAgent := range subAgents {
		names = append(names, subAgent.Name(ctx))
	}
	fmt.Println(strings.Join(names, ", "))

	loopAgent, err := adk.NewLoopAgent(ctx, &adk.LoopAgentConfig{
		Name:          "Roundtable",
		Description:   "多智能体共同讨论并解决问题",
		SubAgents:     subAgents,
		MaxIterations: teamConfig.Roundtable.MaxIterations,
	})
	if err != nil {
		log.Fatal(err)
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           loopAgent,
		EnableStreaming: true,
		CheckPointStore: common.NewInMemoryStore(),
	})

	return runner
}

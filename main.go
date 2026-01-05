package main

import (
	"context"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/leader"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/agents/visitor"
	"fkteams/common"
	"fkteams/fkevent"
	"fkteams/update"
	"fkteams/version"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

func completer(d prompt.Document) []prompt.Suggest {
	if d.GetWordBeforeCursor() == "" {
		return []prompt.Suggest{}
	}
	s := []prompt.Suggest{
		{Text: "quit", Description: "退出"},
		{Text: "help", Description: "帮助信息"},
	}
	return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
}

func main() {

	var (
		checkUpdates bool
		checkVersion bool
		generateEnv  bool
	)
	pflag.BoolVarP(&checkUpdates, "update", "u", false, "检查更新并退出")
	pflag.BoolVarP(&checkVersion, "version", "v", false, "显示版本信息并退出")
	pflag.BoolVarP(&generateEnv, "generate-env", "g", false, "生成示例.env文件并退出")
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

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	fmt.Printf("欢迎来到非空小队: %s\n", version.Get())

	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	coderAgent := coder.NewAgent()
	cmderAgent := cmder.NewAgent()
	leaderAgent := leader.NewAgent()

	subAgents := []adk.Agent{searcherAgent, storytellerAgent, coderAgent, cmderAgent}
	if os.Getenv("FEIKONG_SSH_VISITOR_ENABLED") == "true" {
		visitorAgent := visitor.NewAgent()
		defer visitor.CloseSSHClient()
		subAgents = append(subAgents, visitorAgent)
	}

	ctx, done := context.WithCancel(context.Background())
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

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	inputHistoryPath := "./history/fkteams_history"
	inputHistory, err := common.LoadHistory(inputHistoryPath, 100)
	if err != nil {
		log.Fatalf("加载输入历史失败: %v", err)
	}

	go func() {
		var msgs []adk.Message
		var inputMessages []adk.Message
		for {
			input := prompt.Input("请输入您的问题: ",
				completer,
				prompt.OptionTitle("FeiKong Teams"),
				prompt.OptionPrefixTextColor(prompt.Cyan),
				prompt.OptionSuggestionTextColor(prompt.White),
				prompt.OptionSuggestionBGColor(prompt.Black),
				prompt.OptionDescriptionTextColor(prompt.White),
				prompt.OptionDescriptionBGColor(prompt.Black),
				prompt.OptionHistory(inputHistory),
			)
			if input == "q" || input == "quit" || input == "" {
				pterm.Info.Println("谢谢使用，再见！")
				signals <- syscall.SIGTERM
				return
			}
			if input == "help" {
				pterm.Println("帮助信息: 输入您的问题以获取回答，输入 'quit' 或 'q' 退出程序。")
				continue
			}
			inputHistory = append(inputHistory, input)
			inputMessages = []adk.Message{}
			if len(msgs) > 0 {
				var historyMessage strings.Builder
				for _, msg := range msgs {
					fmt.Fprintf(&historyMessage, "%s: %s\n", msg.Role, msg.Content)
				}
				inputMessages = append(inputMessages, schema.UserMessage(historyMessage.String()))
			}
			inputMessages = append(inputMessages, schema.UserMessage(input))

			iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
			msgs = []adk.Message{}
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

	sig := <-signals
	pterm.Info.Printfln("收到信号: %v, 开始退出前的清理...", sig)

	done()
	err = common.SaveHistory(inputHistoryPath, inputHistory)
	if err != nil {
		log.Fatalf("保存输入历史失败: %v", err)
	}

	pterm.Success.Println("成功退出")
}

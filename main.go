package main

import (
	"context"
	"encoding/hex"
	"fkteams/agents/cmder"
	"fkteams/agents/coder"
	"fkteams/agents/leader"
	"fkteams/agents/searcher"
	"fkteams/agents/storyteller"
	"fkteams/common"
	"fkteams/update"
	"fkteams/version"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

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

func main() {

	var checkUpdates bool
	var checkVersion bool
	pflag.BoolVarP(&checkUpdates, "update", "u", false, "检查更新并退出")
	pflag.BoolVarP(&checkVersion, "version", "v", false, "显示版本信息并退出")
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

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	storytellerAgent := storyteller.NewAgent()
	searcherAgent := searcher.NewAgent()
	coderAgent := coder.NewAgent()
	cmderAgent := cmder.NewAgent()
	leaderAgent := leader.NewAgent()

	ctx, done := context.WithCancel(context.Background())
	supervisorAgent, err := supervisor.New(ctx, &supervisor.Config{
		Supervisor: leaderAgent,
		SubAgents:  []adk.Agent{searcherAgent, storytellerAgent, coderAgent, cmderAgent},
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

	toolTrigger := common.NewOnceWithReset()
	var spinnerLiveText *pterm.SpinnerPrinter

	go func() {
		var msgs, chunks []adk.Message
		var inputMessages []adk.Message
		for {
			input, _ := pterm.DefaultInteractiveTextInput.Show("请输入您的问题")
			if input == "q" || input == "quit" || input == "" {
				pterm.Info.Println("谢谢使用，再见！")
				signals <- syscall.SIGTERM
				return
			}

			// 准备历史信息
			inputMessages = []adk.Message{}
			if len(msgs) > 0 {
				historyMessage := ""
				for _, msg := range msgs {
					historyMessage += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
				}
				inputMessages = append(inputMessages, schema.UserMessage(historyMessage))
			}
			// 添加当前输入
			inputMessages = append(inputMessages, schema.UserMessage(input))

			iter := runner.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams"))
			msgs = []adk.Message{}
			for {
				event, ok := iter.Next()
				if !ok {
					break
				}

				if event.Err != nil {
					log.Println("error:", event.Err)
					continue
				}

				if event.Output.MessageOutput.Role == schema.Tool {
					spinnerLiveText.Success(fmt.Sprintf("[%s]工具调用完成: %s ", event.AgentName, event.Output.MessageOutput.ToolName))
					fmt.Printf("工具返回结果: %s\n", event.Output.MessageOutput.Message.Content)
					fmt.Println()
					continue
				}

				if event.Output.MessageOutput.MessageStream == nil {
					if event.Output.MessageOutput.Message != nil && len(event.Output.MessageOutput.Message.ToolCalls) > 0 {
						for _, toolcall := range event.Output.MessageOutput.Message.ToolCalls {
							if toolcall.Function.Name == "transfer_to_agent" {
								fmt.Printf("\n[%s] ==> [%s]\n\n", event.AgentName, toolcall.Function.Arguments)
							}
						}
					}
					continue
				}

				toolTrigger.Reset()
				chunks = []adk.Message{}

				for {
					chunk, err := event.Output.MessageOutput.MessageStream.Recv()
					if err != nil {
						if err == io.EOF {
							break
						}
						log.Fatal(err)
					}

					if len(chunk.ToolCalls) > 0 {
						for _, tc := range chunk.ToolCalls {
							toolTrigger.Do(func() {
								fmt.Println()
								spinnerLiveText, _ = pterm.DefaultSpinner.Start("正在准备工具调用参数...")
							})
							spinnerLiveText.UpdateText(fmt.Sprintf("正在准备工具调用参数...%s", hex.EncodeToString([]byte(tc.Function.Arguments))))
						}
					}

					if chunk.Content != "" {
						fmt.Print(chunk.Content)
					}

					chunks = append(chunks, chunk)

				}
				fmt.Println()
				// 记录历史信息
				if len(chunks) > 0 {
					concatMessages, err := common.ConcatMessages(chunks)
					if err != nil {
						pterm.Error.Printfln("failed to concat messages: %v", err)
						continue
					}
					msgs = append(msgs, concatMessages)
				}
			}
		}
	}()

	sig := <-signals
	pterm.Info.Printfln("收到信号: %v, 开始退出前的清理...", sig)
	done()
	pterm.Success.Println("成功退出")
}

package main

import (
	"context"
	"fkteams/cli"
	"fkteams/common"
	"fkteams/config"
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

	"github.com/cloudwego/eino/adk"
	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	"github.com/spf13/pflag"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
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
	pflag.StringVarP(&workMode, "work-mode", "m", "team", "工作模式: team 或 deep 或 group 或 custom")
	pflag.Parse()

	if checkVersion {
		fmt.Printf("fkteams: %s\n", version.Get())
		return
	}

	if generateEnv {
		if err := common.GenerateExampleEnv(".env.example"); err != nil {
			log.Fatal(err)
		}
		fmt.Println("成功生成示例.env文件: .env.example")
		return
	}

	if generateConfig {
		if err := config.GenerateExample(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("成功生成示例配置文件: config/config.toml")
		return
	}

	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		fmt.Println("加载 .env 文件失败，请确保已创建该文件")
		fmt.Println("可以使用 --generate-env 或者 -g 参数生成示例文件")
		return
	}

	if checkUpdates {
		if err := update.SelfUpdate("wsshow", "feikong-teams"); err != nil {
			log.Fatal(err)
		}
		return
	}

	if web {
		server.Run()
		return
	}

	ctx, done := context.WithCancel(context.Background())
	currentMode := cli.ParseWorkMode(workMode)

	var r *adk.Runner
	switch currentMode {
	case cli.ModeTeam:
		r = createModeRunner(ctx, cli.ModeTeam)
	case cli.ModeGroup:
		r = createModeRunner(ctx, cli.ModeGroup)
	case cli.ModeCustom:
		r = createModeRunner(ctx, cli.ModeCustom)
	case cli.ModeDeep:
		r = createModeRunner(ctx, cli.ModeDeep)
	default:
		pterm.Error.Println("暂不支持该模式：", workMode)
		return
	}

	defer func() {
		if err := g.Cleaner.ExecuteAndClear(); err != nil {
			log.Fatalf("清理资源失败: %v", err)
		}
	}()

	// 加载输入历史
	inputHistoryPath := "./history/input_history/fkteams_input_history"
	inputHistory, err := common.LoadHistory(inputHistoryPath, 100)
	if err != nil {
		log.Fatalf("加载输入历史失败: %v", err)
	}

	// 创建交互会话
	session := cli.NewSession(currentMode, inputHistory, createModeRunner)

	// 信号处理
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	exitSignals := make(chan os.Signal, 1)
	session.StartSignalHandler(signals, exitSignals)

	if query != "" {
		session.HandleDirect(ctx, r, exitSignals, query)
	} else {
		session.HandleInteractive(ctx, r, exitSignals)
	}

	sig := <-exitSignals
	pterm.Info.Printfln("收到信号: %v, 开始退出前的清理...", sig)
	done()

	if err := common.SaveHistory(inputHistoryPath, session.InputHistory); err != nil {
		log.Fatalf("保存输入历史失败: %v", err)
	}

	pterm.Success.Println("成功退出")
}

// createModeRunner 根据工作模式创建对应的 Runner
func createModeRunner(ctx context.Context, mode cli.WorkMode) *adk.Runner {
	switch mode {
	case cli.ModeTeam:
		fmt.Printf("欢迎来到非空小队: %s\n", version.Get())
		return runner.CreateSupervisorRunner(ctx)
	case cli.ModeDeep:
		fmt.Printf("欢迎来到非空小队 - 深度模式: %s\n", version.Get())
		return runner.CreateDeepAgentsRunner(ctx)
	case cli.ModeGroup:
		fmt.Printf("欢迎来到非空小队 - 多智能体讨论模式: %s\n", version.Get())
		runner.PrintLoopAgentsInfo(ctx)
		return runner.CreateLoopAgentRunner(ctx)
	case cli.ModeCustom:
		fmt.Printf("欢迎来到非空小队 - 自定义会议模式: %s\n", version.Get())
		runner.PrintCustomAgentsInfo(ctx)
		return runner.CreateCustomSupervisorRunner(ctx)
	default:
		return nil
	}
}

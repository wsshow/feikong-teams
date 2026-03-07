package commands

import (
	"context"
	"fkteams/agents/common"
	"fkteams/cli"
	commonPkg "fkteams/common"
	"fkteams/g"
	"fkteams/memory"
	"fkteams/runner"
	"fkteams/tools/scheduler"
	"fkteams/version"
	"fmt"
	"log"
	"os"

	"github.com/cloudwego/eino/adk"
	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	ucli "github.com/urfave/cli/v3"
)

// chatAction 默认操作：启动交互模式或直接查询模式
func chatAction(ctx context.Context, cmd *ucli.Command) error {
	if err := godotenv.Load(); err != nil {
		fmt.Println("加载 .env 文件失败，请确保已创建该文件")
		fmt.Println("可以使用 generate env 子命令生成示例文件")
		return nil
	}

	appCtx, done := context.WithCancel(ctx)
	workMode := cmd.String("mode")
	currentMode := cli.ParseWorkMode(workMode)

	var r *adk.Runner
	switch currentMode {
	case cli.ModeTeam:
		r = createModeRunner(appCtx, cli.ModeTeam)
	case cli.ModeGroup:
		r = createModeRunner(appCtx, cli.ModeGroup)
	case cli.ModeCustom:
		r = createModeRunner(appCtx, cli.ModeCustom)
	case cli.ModeDeep:
		r = createModeRunner(appCtx, cli.ModeDeep)
	default:
		pterm.Error.Println("暂不支持该模式：", workMode)
		done()
		return nil
	}

	defer func() {
		if err := g.Cleaner.ExecuteAndClear(); err != nil {
			log.Fatalf("清理资源失败: %v", err)
		}
	}()

	// 初始化全局记忆管理器
	workspaceDir := "./workspace"
	if d := os.Getenv("FEIKONG_WORKSPACE_DIR"); d != "" {
		workspaceDir = d
	}
	if os.Getenv("FEIKONG_MEMORY_ENABLED") == "true" {
		g.MemManager = memory.NewManager(workspaceDir, memory.NewLLMClient(common.NewChatModel()))
		pterm.Info.Println("全局长期记忆已启用")
	}

	// 启动定时任务调度器
	if s := scheduler.Global(); s != nil {
		outputDir := "./result/scheduled_tasks/"
		executor := scheduler.NewBackgroundExecutor(func(ctx context.Context) *adk.Runner {
			return runner.CreateBackgroundTaskRunner(ctx)
		}, outputDir)
		s.SetExecutor(executor)
		s.Start()
		g.Cleaner.Add(func() error {
			s.Stop()
			return nil
		})
	}

	// 加载输入历史
	inputHistoryPath := "./history/input_history/fkteams_input_history"
	inputHistory, err := commonPkg.LoadHistory(inputHistoryPath, 100)
	if err != nil {
		log.Fatalf("加载输入历史失败: %v", err)
	}

	// 创建交互会话
	session := cli.NewSession(currentMode, inputHistory, createModeRunner)

	// 设置恢复会话
	if resumeSession := cmd.String("resume"); resumeSession != "" {
		cli.SetResumeSessionID(resumeSession)
	}

	// 信号处理：SIGINT 由 Session 内部处理（bubbletea 捕获输入时、信号处理器捕获查询时）
	exitSignals := make(chan os.Signal, 1)
	session.StartSignalHandler(exitSignals)

	query := cmd.String("query")
	if query != "" {
		session.HandleDirect(appCtx, r, exitSignals, query)
	} else {
		session.HandleInteractive(appCtx, r, exitSignals)
	}

	sig := <-exitSignals
	pterm.Info.Printfln("收到信号: %v, 开始退出前的清理...", sig)
	done()

	if err := commonPkg.SaveHistory(inputHistoryPath, session.InputHistory); err != nil {
		log.Fatalf("保存输入历史失败: %v", err)
	}

	pterm.Success.Println("成功退出")
	return nil
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

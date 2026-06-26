// Package httpserver 提供 Web 服务端模式的入口和 HTTP 服务管理。
package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	channel "fkteams/internal/adapters/transport/channel"
	_ "fkteams/internal/adapters/transport/channel/discord"
	_ "fkteams/internal/adapters/transport/channel/qq"
	_ "fkteams/internal/adapters/transport/channel/weixin"
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/adapters/transport/http/router"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/config"
	"fkteams/internal/app/lifecycle"
	"fkteams/internal/app/version"
	bootstrapservices "fkteams/internal/bootstrap/services"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/log"

	"github.com/gin-gonic/gin"
)

// serverMode 服务模式
type serverMode int

const (
	ModeWeb serverMode = iota // 含 Web 界面
	ModeAPI                   // 纯 API 服务
)

// httpService HTTP 服务，实现 lifecycle.Service 接口
type httpService struct {
	host          string     // 监听地址
	port          int        // 监听端口
	logLevel      string     // 日志级别
	mode          serverMode // 服务模式
	state         *appstate.State
	resetChannels func()
	runtime       *handler.Runtime
	server        *http.Server // HTTP 服务实例
}

// Name 返回服务名称
func (s *httpService) Name() string { return "http" }

// Start 启动 HTTP 服务（非阻塞）
func (s *httpService) Start(ctx context.Context) error {
	if s.logLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	port := s.port
	if port <= 0 {
		port = 23456
	}

	var (
		h   http.Handler
		err error
	)
	engine, _ := runtimeport.EngineFromContext(ctx)
	interrupt, _ := runtimeport.InterruptRuntimeFromContext(ctx)
	s.runtime = handler.NewRuntime(handler.RuntimeOptions{
		Engine:        engine,
		Interrupt:     interrupt,
		ResetChannels: s.resetChannels,
	})
	if s.mode == ModeAPI {
		h, err = router.InitAPIWithRuntime(s.state, s.runtime)
	} else {
		h, err = router.InitWithRuntime(s.state, s.runtime)
	}
	if err != nil {
		return fmt.Errorf("init router: %w", err)
	}

	s.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", s.host, port),
		Handler:        h,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}
	s.server.RegisterOnShutdown(s.runtime.Connections.CloseAllWebSockets)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[http] server error: %v", err)
		}
	}()

	log.Printf("[http] 服务运行在 [%s]", s.server.Addr)
	return nil
}

// Stop 优雅关闭 HTTP 服务（5 秒超时）
func (s *httpService) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Println("[http] 正在关闭 HTTP 服务...")
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown error: %w", err)
	}
	log.Println("[http] HTTP 服务已关闭")
	return nil
}

// Addr 返回服务监听地址
func (s *httpService) Addr() string {
	if s.server != nil {
		return s.server.Addr
	}
	return ""
}

// ServeOptions serve 命令的配置选项
type ServeOptions struct {
	Host string
	Port int
}

// run 启动服务的公共逻辑。
func run(ctx context.Context, mode serverMode, opts *ServeOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := config.Get()

	app := lifecycle.New()
	appCfg := app.Config()
	state := app.State()

	// 注册关闭回调供 API 调用
	lifecycle.SetShutdownFunc(func() { app.Shutdown() })

	if appCfg.MemoryEnabled {
		app.RegisterService(bootstrapservices.NewMemoryService(appCfg.WorkspaceDir, state))
	}
	if appCfg.SchedulerEnabled {
		app.RegisterService(bootstrapservices.NewSchedulerService(appCfg.SchedulerDir))
	}

	host := "127.0.0.1"
	if cfg.Server.Host != "" {
		host = cfg.Server.Host
	}
	port := cfg.Server.Port
	if opts != nil {
		if opts.Host != "" {
			host = opts.Host
		}
		if opts.Port > 0 {
			port = opts.Port
		}
	}

	httpSvc := &httpService{
		host:     host,
		port:     port,
		logLevel: cfg.Server.LogLevel,
		mode:     mode,
		state:    state,
	}
	app.RegisterService(httpSvc)

	// 注册消息通道
	if svc, err := channel.SetupWithState(cfg.Channels.List(), state); err != nil {
		return fmt.Errorf("setup channels: %w", err)
	} else if svc != nil {
		httpSvc.resetChannels = svc.ResetRunners
		app.RegisterService(svc)
	}

	app.OnReady(func(ctx context.Context) error {
		addr := httpSvc.Addr()
		if mode == ModeAPI {
			fmt.Printf("欢迎来到非空小队 - API 服务模式: %s\n", version.Get())
			fmt.Printf("当前服务运行在 [%s]\n", addr)
		} else {
			fmt.Printf("欢迎来到非空小队 - 服务端模式: %s\n", version.Get())
			fmt.Printf("当前服务运行在 [%s]\n", addr)
			fmt.Printf("前端页面地址: http://%s\n", addr)
		}
		if appCfg.MemoryEnabled {
			fmt.Println("全局长期记忆已启用")
		}
		return nil
	})

	app.OnCleanup(func(ctx context.Context) error {
		state.RunProcessCleanup()

		// 如有待重启请求，启动新进程
		if err := lifecycle.ExecutePendingRestart(); err != nil {
			log.Printf("[server] restart failed: %v", err)
		}

		fmt.Printf("服务安全退出\n")
		return nil
	})

	if err := app.Run(ctx); err != nil {
		return fmt.Errorf("application error: %w", err)
	}
	return nil
}

// Run 启动 Web 服务器模式
func Run() error {
	return RunContext(context.Background())
}

// RunContext 使用显式 context 启动 Web 服务器模式。
func RunContext(ctx context.Context) error {
	return run(ctx, ModeWeb, nil)
}

// RunServe 启动纯 API 服务（无 Web 界面）
func RunServe(opts ServeOptions) error {
	return RunServeContext(context.Background(), opts)
}

// RunServeContext 使用显式 context 启动纯 API 服务（无 Web 界面）。
func RunServeContext(ctx context.Context, opts ServeOptions) error {
	return run(ctx, ModeAPI, &opts)
}

// Package server 提供 Web 服务端模式的入口和 HTTP 服务管理
package server

import (
	"context"
	"fkteams/channels"
	_ "fkteams/channels/discord"
	_ "fkteams/channels/qq"
	"fkteams/config"
	"fkteams/lifecycle"
	"fkteams/log"
	"fkteams/server/handler"
	"fkteams/server/router"
	"fkteams/version"
	"fmt"
	"net/http"
	"time"

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
	host     string       // 监听地址
	port     int          // 监听端口
	logLevel string       // 日志级别
	mode     serverMode   // 服务模式
	server   *http.Server // HTTP 服务实例
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
	if s.mode == ModeAPI {
		h, err = router.InitAPI()
	} else {
		h, err = router.Init()
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
	s.server.RegisterOnShutdown(handler.CloseAllWebSockets)

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

// run 启动服务的公共逻辑
func run(mode serverMode, opts *ServeOptions) error {
	cfg, err := config.Get()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	app := lifecycle.New()
	appCfg := app.Config()

	if appCfg.MemoryEnabled {
		app.RegisterService(lifecycle.NewMemoryService(appCfg.WorkspaceDir))
	}
	if appCfg.SchedulerEnabled {
		app.RegisterService(lifecycle.NewSchedulerService(appCfg.WorkspaceDir, appCfg.SchedulerOutputDir))
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
	}
	app.RegisterService(httpSvc)

	// 注册消息通道
	if svc, err := channels.Setup(cfg.Channels.List()); err != nil {
		return fmt.Errorf("setup channels: %w", err)
	} else if svc != nil {
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
		fmt.Printf("服务安全退出\n")
		return nil
	})

	if err := app.Run(context.Background()); err != nil {
		return fmt.Errorf("application error: %w", err)
	}
	return nil
}

// Run 启动 Web 服务器模式
func Run() error {
	return run(ModeWeb, nil)
}

// RunServe 启动纯 API 服务（无 Web 界面）
func RunServe(opts ServeOptions) error {
	return run(ModeAPI, &opts)
}

package server

import (
	"context"
	"fkteams/config"
	"fkteams/lifecycle"
	"fkteams/server/handler"
	"fkteams/server/router"
	"fkteams/version"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// httpService HTTP 服务，实现 lifecycle.Service 接口
type httpService struct {
	port     int
	logLevel string
	server   *http.Server
}

func newHttpService(port int, logLevel string) *httpService {
	return &httpService{
		port:     port,
		logLevel: logLevel,
	}
}

func (s *httpService) Name() string { return "http" }

func (s *httpService) Start(ctx context.Context) error {
	if s.logLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	port := s.port
	if port <= 0 {
		port = 23456
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router.Init(),
	}
	s.server.RegisterOnShutdown(handler.CloseAllWebSockets)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[http] server error: %v", err)
		}
	}()

	log.Printf("[http] 服务运行在端口 %s", s.server.Addr)
	return nil
}

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

func (s *httpService) Addr() string {
	if s.server != nil {
		return s.server.Addr
	}
	return ""
}

// Run 启动 Web 服务器模式
func Run() {
	cfg, err := config.Get()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	app := lifecycle.New()
	appCfg := app.Config()

	if appCfg.MemoryEnabled {
		app.RegisterService(lifecycle.NewMemoryService(appCfg.WorkspaceDir))
	}
	if appCfg.SchedulerEnabled {
		app.RegisterService(lifecycle.NewSchedulerService(appCfg.WorkspaceDir, appCfg.SchedulerOutputDir))
	}

	httpSvc := newHttpService(cfg.Server.Port, cfg.Server.LogLevel)
	app.RegisterService(httpSvc)

	app.OnReady(func(ctx context.Context) error {
		addr := httpSvc.Addr()
		fmt.Printf("欢迎来到非空小队 - 服务端模式: %s\n", version.Get())
		fmt.Printf("当前服务运行在端口 [%s]\n", addr)
		fmt.Printf("前端页面地址: http://localhost%s\n", addr)
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
		log.Fatalf("application error: %v", err)
	}
}

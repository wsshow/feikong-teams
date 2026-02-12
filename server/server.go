package server

import (
	"context"
	"fkteams/config"
	"fkteams/server/handler"
	"fkteams/server/router"
	"fkteams/version"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func Run() {
	cfg, err := config.Get()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if cfg.Server.LogLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	port := cfg.Server.Port
	if port <= 0 {
		port = 23456
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router.Init(),
	}
	srv.RegisterOnShutdown(handler.CloseAllWebSockets)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	fmt.Printf("欢迎来到非空小队 - 服务端模式: %s\n", version.Get())
	fmt.Printf("当前服务运行在端口 [%s]\n", srv.Addr)
	fmt.Printf("前端页面地址: http://localhost%s\n", srv.Addr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("服务安全退出 %s\n", srv.Addr)
}

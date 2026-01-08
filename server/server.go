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

func initHttpServer(port int, handler http.Handler) *http.Server {
	if port <= 0 {
		port = 23456
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	return srv
}

func Run() {
	cfg, err := config.Get()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if cfg.Server.LogLevel != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	apiSrv, sProtocol := func() (*http.Server, string) {
		srv := initHttpServer(cfg.Server.Port, router.Init())
		// 服务退出时主动断开所有 websocket 长连接
		srv.RegisterOnShutdown(handler.CloseAllWebSockets)
		return srv, "http"
	}()
	fmt.Printf("欢迎来到非空小队 - 服务端模式: %s\n", version.Get())
	fmt.Printf("当前服务运行在端口 [%s]\n", apiSrv.Addr)
	fmt.Printf("前端页面地址: http://localhost%s\n", apiSrv.Addr)
	signalExit := make(chan os.Signal, 1)
	signal.Notify(signalExit, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	<-signalExit
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := apiSrv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("服务安全退出 %s, 协议: %s\n", apiSrv.Addr, sProtocol)
}

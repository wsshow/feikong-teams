// Package upgrade 提供启动业务服务之前的存储升级界面和控制接口。
package upgrade

import (
	"context"
	"errors"
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/adapters/transport/http/middleware"
	"fkteams/internal/app/config"
	appmigration "fkteams/internal/app/migration"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"
)

// Serve 在受认证保护的维护入口中等待升级完成，期间不启动业务服务。
func Serve(ctx context.Context, address string, assets fs.FS, service *appmigration.Service) error {
	router := newRouter(ctx, assets, service)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 10 * time.Second}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	defer server.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-finished:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("upgrade server: %w", err)
		case <-ticker.C:
			if service.Status().State == "completed" {
				shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return server.Shutdown(shutdown)
			}
		}
	}
}

func newRouter(ctx context.Context, assets fs.FS, service *appmigration.Service) *gin.Engine {
	router := gin.New()
	if err := router.SetTrustedProxies(config.Get().Server.TrustedProxies); err != nil {
		_ = router.SetTrustedProxies(nil)
	}
	router.Use(gin.Recovery(), middleware.Cors(), middleware.Auth())
	router.POST("/api/fkteams/login", middleware.MaxBodySize(16<<10), handler.LoginHandler())
	router.POST("/api/fkteams/logout", handler.LogoutHandler())
	router.GET("/api/fkteams/storage/migration", func(c *gin.Context) { handler.OK(c, gin.H{"migration": service.Status(), "ready": false}) })
	router.POST("/api/fkteams/storage/migration", func(c *gin.Context) {
		service.Start(ctx)
		handler.OK(c, gin.H{"migration": service.Status(), "ready": false})
	})
	router.GET("/live", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/ready", func(c *gin.Context) { c.Status(http.StatusServiceUnavailable) })
	files := http.FileServer(http.FS(assets))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/v1/") || c.Request.URL.Path == "/ws" {
			handler.Fail(c, http.StatusServiceUnavailable, "正在升级数据，请稍后重试")
			return
		}
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") {
			files.ServeHTTP(c.Writer, c.Request)
			return
		}
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			handler.Fail(c, 500, "页面加载失败")
			return
		}
		c.Data(200, "text/html; charset=utf-8", data)
	})
	return router
}

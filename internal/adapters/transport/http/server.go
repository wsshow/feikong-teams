// Package httpserver 提供 Web 服务端模式的入口和 HTTP 服务管理。
package httpserver

import (
	"context"
	"errors"
	"fkteams/internal/adapters/transport/http/handler"
	"fkteams/internal/adapters/transport/http/router"
	"fkteams/internal/app/appstate"
	appschedule "fkteams/internal/app/schedule"
	"fkteams/internal/runtime/log"
	"fmt"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Mode 描述 HTTP 服务是否提供 Web 界面。
type Mode int

const (
	ModeWeb Mode = iota // 含 Web 界面
	ModeAPI             // 纯 API 服务
)

// Service HTTP 服务，实现 lifecycle.Service 接口
type Service struct {
	host              string // 监听地址
	port              int    // 监听端口
	logLevel          string // 日志级别
	mode              Mode   // 服务模式
	state             *appstate.State
	schedulerProvider func() *appschedule.Service
	runtime           *handler.Runtime
	server            *http.Server // HTTP 服务实例
	listener          net.Listener
	serveDone         chan error
	listen            func(network, address string) (net.Listener, error)
}

// Name 返回服务名称
func (s *Service) Name() string { return "http" }

// Start 启动 HTTP 服务（非阻塞）
func (s *Service) Start(ctx context.Context) error {
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
	if s.runtime == nil {
		s.runtime = handler.NewRuntime()
	}
	if s.schedulerProvider != nil {
		s.runtime.Scheduler = s.schedulerProvider()
	}
	if s.mode == ModeAPI {
		h, err = router.InitAPIWithRuntime(s.state, s.runtime)
	} else {
		h, err = router.InitWithRuntime(s.state, s.runtime)
	}
	if err != nil {
		s.runtime.Close()
		return fmt.Errorf("init router: %w", err)
	}

	s.server = &http.Server{
		Addr:           net.JoinHostPort(s.host, strconv.Itoa(port)),
		Handler:        h,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}
	s.server.RegisterOnShutdown(s.runtime.Connections.CloseAllWebSockets)

	listen := s.listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", s.server.Addr)
	if err != nil {
		s.runtime.Close()
		return fmt.Errorf("listen on %s: %w", s.server.Addr, err)
	}
	s.listener = listener
	s.server.Addr = listener.Addr().String()
	if err := s.runtime.Start(ctx); err != nil {
		_ = listener.Close()
		s.runtime.Close()
		return fmt.Errorf("start HTTP runtime: %w", err)
	}

	s.serveDone = make(chan error, 1)
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[http] server error: %v", err)
		}
		s.serveDone <- err
		close(s.serveDone)
	}()

	log.Printf("[http] 服务运行在 [%s]", s.server.Addr)
	return nil
}

// Stop 优雅关闭 HTTP 服务（5 秒超时）
func (s *Service) Stop(ctx context.Context) error {
	var result error
	if s.runtime != nil {
		s.runtime.BeginShutdown()
	}
	if s.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			closeErr := s.server.Close()
			if closeErr != nil {
				result = fmt.Errorf("http shutdown error: %w", errors.Join(err, closeErr))
			} else {
				result = fmt.Errorf("http shutdown error: %w", err)
			}
		} else if s.serveDone != nil {
			select {
			case err := <-s.serveDone:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					result = fmt.Errorf("http serve error: %w", err)
				}
			case <-shutdownCtx.Done():
				result = fmt.Errorf("wait for HTTP server stop: %w", shutdownCtx.Err())
			}
		}
	}
	if s.runtime != nil {
		result = errors.Join(result, s.runtime.Shutdown(ctx))
	}
	return result
}

// Addr 返回服务监听地址
func (s *Service) Addr() string {
	if s.server != nil {
		return s.server.Addr
	}
	return ""
}

// ServeOptions 描述 HTTP 服务的监听选项。
type ServeOptions struct {
	Host string
	Port int
}

// ServiceOptions 是 HTTP 传输层需要的显式依赖，不负责创建应用及后台服务。
type ServiceOptions struct {
	Listen            ServeOptions
	Mode              Mode
	LogLevel          string
	State             *appstate.State
	Runtime           *handler.Runtime
	SchedulerProvider func() *appschedule.Service
}

// NewService 创建 HTTP 服务，由组合根注册到应用生命周期。
func NewService(opts ServiceOptions) *Service {
	return &Service{host: opts.Listen.Host, port: opts.Listen.Port, mode: opts.Mode, logLevel: opts.LogLevel, state: opts.State, runtime: opts.Runtime, schedulerProvider: opts.SchedulerProvider}
}

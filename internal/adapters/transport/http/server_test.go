package httpserver

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"fkteams/internal/app/appstate"
)

type blockingListener struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingListener() *blockingListener {
	return &blockingListener{closed: make(chan struct{})}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, net.ErrClosed
}

func (l *blockingListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *blockingListener) Addr() net.Addr {
	return testAddr("127.0.0.1:34567")
}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

func TestHTTPServiceStartReturnsListenError(t *testing.T) {
	wantErr := errors.New("address unavailable")
	service := &httpService{
		mode:  ModeAPI,
		state: appstate.New(),
		listen: func(string, string) (net.Listener, error) {
			return nil, wantErr
		},
	}

	err := service.Start(context.Background())
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "listen on") {
		t.Fatalf("Start() error = %v, want listen error", err)
	}
}

func TestHTTPServiceUsesBoundAddressAndWaitsForServeExit(t *testing.T) {
	listener := newBlockingListener()
	service := &httpService{
		mode:  ModeAPI,
		state: appstate.New(),
		listen: func(string, string) (net.Listener, error) {
			return listener, nil
		},
	}

	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	if got := service.Addr(); got != listener.Addr().String() {
		t.Fatalf("Addr() = %q, want %q", got, listener.Addr().String())
	}
	if err := service.Stop(context.Background()); err != nil {
		t.Fatalf("Stop(): %v", err)
	}
}

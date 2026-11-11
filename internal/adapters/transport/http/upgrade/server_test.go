package upgrade

import (
	"context"
	"fkteams/internal/app/config"
	appmigration "fkteams/internal/app/migration"
	domainmigration "fkteams/internal/domain/migration"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

type blockedSource struct {
	release chan struct{}
	calls   atomic.Int32
}

func (s *blockedSource) Prepare(ctx context.Context, _ func(string, int, int)) (domainmigration.Snapshot, error) {
	s.calls.Add(1)
	select {
	case <-s.release:
		return domainmigration.Snapshot{}, nil
	case <-ctx.Done():
		return domainmigration.Snapshot{}, ctx.Err()
	}
}

type target struct{}

func (*target) Completed(context.Context) (bool, error) { return false, nil }
func (*target) Import(context.Context, domainmigration.Snapshot, func(string, int, int)) error {
	return nil
}

func TestMaintenanceBlocksBusinessAndKeepsJobAfterDisconnect(t *testing.T) {
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	source := &blockedSource{release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service, err := appmigration.NewService(ctx, source, &target{})
	if err != nil {
		t.Fatal(err)
	}
	router := newRouter(ctx, fstest.MapFS{"index.html": {Data: []byte("upgrade app")}}, service)
	request := func(method, path, origin string, requestCtx context.Context) int {
		r := httptest.NewRequest(method, "http://localhost"+path, nil).WithContext(requestCtx)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	if got := request(http.MethodPost, "/api/fkteams/chat", "", ctx); got != 503 {
		t.Fatalf("business endpoint available: %d", got)
	}
	if got := request(http.MethodGet, "/ready", "", ctx); got != 503 {
		t.Fatalf("maintenance ready: %d", got)
	}
	if got := request(http.MethodPost, "/api/fkteams/storage/migration", "https://untrusted.example", ctx); got != 403 {
		t.Fatalf("cross origin migration allowed: %d", got)
	}
	requestCtx, disconnect := context.WithCancel(ctx)
	if got := request(http.MethodPost, "/api/fkteams/storage/migration", "", requestCtx); got != 200 {
		t.Fatal(got)
	}
	disconnect()
	if got := request(http.MethodPost, "/api/fkteams/storage/migration", "", ctx); got != 200 {
		t.Fatal(got)
	}
	close(source.release)
	waitCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if err := service.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if source.calls.Load() != 1 {
		t.Fatal("duplicate job")
	}
	cfg := *config.Get()
	cfg.Server.Auth.Enabled = true
	cfg.Server.Auth.Secret = "unit-test-signing-secret-not-used-in-production"
	cfg.Server.Auth.Username = "test-user"
	cfg.Server.Auth.Password = "test-only-password"
	if err := config.Save(&cfg); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPost, "/api/fkteams/storage/migration", "", ctx); got != 401 {
		t.Fatalf("unauthenticated migration allowed: %d", got)
	}
}

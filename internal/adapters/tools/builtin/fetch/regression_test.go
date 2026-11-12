package fetch

import (
	"context"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestFetchToolReusesConnectionsAcrossCalls(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "body") }))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	tools, err := GetTools()
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		resp, err := tools[0].Invoke(context.Background(), runtimeport.ToolInvocation{Arguments: fmt.Sprintf(`{"url":%q}`, server.URL)})
		if err != nil || !strings.Contains(resp.Content, `"content":"body"`) {
			t.Fatalf("fetch: %#v %v", resp, err)
		}
	}
	if connections.Load() != 1 {
		t.Fatalf("connection pool was not reused: %d", connections.Load())
	}
}
func TestTruncationDoesNotHideInteriorInvalidUTF8(t *testing.T) {
	content := strings.Repeat("a", 64<<10) + "\xff" + strings.Repeat("b", 64<<10)
	body, truncated, err := readResponseBody(strings.NewReader(content), 100<<10)
	if err != nil || !truncated || utf8.Valid(body) {
		t.Fatalf("interior encoding error hidden: len=%d truncated=%v err=%v", len(body), truncated, err)
	}
}

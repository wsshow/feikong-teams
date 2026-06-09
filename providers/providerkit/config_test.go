package providerkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListOpenAIModelsRequestsModelsEndpoint(t *testing.T) {
	var gotAuth string
	var gotExtra string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("request path = %q, want /v1/models", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotExtra = r.Header.Get("X-Gateway")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "gpt-5"},
				{"id": "gpt-5-mini"},
			},
		})
	}))
	defer server.Close()

	models, err := ListOpenAIModels(context.Background(), &Config{
		BaseURL:      server.URL + "/v1/",
		APIKey:       "sk-test",
		ExtraHeaders: map[string]string{"X-Gateway": "token"},
	})
	if err != nil {
		t.Fatalf("ListOpenAIModels() error = %v", err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
	if gotExtra != "token" {
		t.Fatalf("X-Gateway = %q, want token", gotExtra)
	}
	if len(models) != 2 || models[0].ID != "gpt-5" || models[1].ID != "gpt-5-mini" {
		t.Fatalf("models = %v, want two model IDs", models)
	}
}

func TestListOpenAIModelsValidatesConfigAndStatus(t *testing.T) {
	if _, err := ListOpenAIModels(context.Background(), &Config{}); err == nil || !strings.Contains(err.Error(), "未配置 base_url") {
		t.Fatalf("empty base_url error = %v", err)
	}

	tests := []struct {
		name       string
		statusCode int
		want       string
	}{
		{name: "not found", statusCode: http.StatusNotFound, want: "不支持模型列表查询"},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, want: "API Key 认证失败"},
		{name: "forbidden", statusCode: http.StatusForbidden, want: "API Key 认证失败"},
		{name: "server error", statusCode: http.StatusInternalServerError, want: "模型列表请求返回状态 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			_, err := ListOpenAIModels(context.Background(), &Config{BaseURL: server.URL})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ListOpenAIModels() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestListOpenAIModelsReportsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()

	_, err := ListOpenAIModels(context.Background(), &Config{BaseURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "解析模型列表失败") {
		t.Fatalf("ListOpenAIModels() error = %v, want parse error", err)
	}
}

func TestNewHTTPClientUsesProxyEnvWhenValid(t *testing.T) {
	proxy := "http://127.0.0.1:7890"
	t.Setenv("FEIKONG_PROXY_URL", proxy)

	client := NewHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", client.Transport)
	}
	req := httptest.NewRequest("GET", "http://example.com", nil)
	got, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	want, _ := url.Parse(proxy)
	if got == nil || got.String() != want.String() {
		t.Fatalf("proxy = %v, want %v", got, want)
	}
}

func TestHTTPClientWithHeaders(t *testing.T) {
	if got := HTTPClientWithHeaders(nil); got != nil {
		t.Fatalf("HTTPClientWithHeaders(nil) = %#v, want nil", got)
	}

	client := HTTPClientWithHeaders(map[string]string{"X-Test": "yes"})
	if client == nil {
		t.Fatal("HTTPClientWithHeaders(non-empty) returned nil")
	}
	transport, ok := client.Transport.(*headerTransport)
	if !ok {
		t.Fatalf("transport = %T, want *headerTransport", client.Transport)
	}
	req := httptest.NewRequest("GET", "http://example.com", nil)
	transport.base = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Test") != "yes" {
			t.Fatalf("X-Test header = %q, want yes", req.Header.Get("X-Test"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       http.NoBody,
			Header:     http.Header{},
			Request:    req,
		}, nil
	})
	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

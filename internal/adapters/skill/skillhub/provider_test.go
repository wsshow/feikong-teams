package skillhub

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderSearchAndDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/skills":
			if r.URL.Query().Get("keyword") != "go" || r.URL.Query().Get("page") != "2" {
				t.Fatalf("unexpected query: %s", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"data":{"skills":[{"name":"Go","slug":"go","ownerName":"owner"}],"total":1}}`)
		case "/api/download":
			if r.URL.Query().Get("slug") != "go" {
				t.Fatalf("unexpected slug: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, "archive")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := New(server.URL+"/api/skills", server.Client())
	result, err := provider.Search(context.Background(), "go", 2, 20, "downloads", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Skills) != 1 || result.Skills[0].Slug != "go" {
		t.Fatalf("unexpected result: %#v", result)
	}

	body, err := provider.Download(context.Background(), "go", "")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != "archive" {
		t.Fatalf("unexpected download: %q err=%v", data, err)
	}
}

func TestProviderRejectsUpstreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusBadGateway)
	}))
	defer server.Close()
	provider := New(server.URL, server.Client())
	if _, err := provider.Search(context.Background(), "go", 1, 20, "", ""); err == nil {
		t.Fatal("expected upstream error")
	}
}

func TestDecodeSearchResponseRejectsOversizedBody(t *testing.T) {
	reader := io.LimitReader(skillFillReader{}, maxSearchResponseBytes+1)
	if err := decodeSearchResponse(reader, &struct{}{}); err == nil {
		t.Fatal("oversized skill search response was accepted")
	}
}

type skillFillReader struct{}

func (skillFillReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = '0'
	}
	return len(buffer), nil
}

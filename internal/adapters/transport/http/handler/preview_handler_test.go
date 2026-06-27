package handler

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPreviewLinkHandlersLifecycle(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*previewLinkEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.MkdirAll(filepath.Join(workspace, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "docs", "report.txt"), []byte("report body"), 0644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	router := gin.New()
	router.POST("/preview-links", rt.CreatePreviewLinkHandler())
	router.GET("/preview-links", rt.ListPreviewLinksHandler())
	router.GET("/preview/:linkId/info", rt.PreviewInfoHandler())
	router.GET("/preview/:linkId/file", rt.PreviewFileHandler())
	router.DELETE("/preview/:linkId", rt.DeletePreviewLinkHandler())

	resp := performJSON(router, http.MethodPost, "/preview-links", `{"file_path":"docs/report.txt","expires_in":-1}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("create preview link status = %d: %s", resp.Code, resp.Body.String())
	}
	var link PreviewLink
	decodeRawData(t, resp, &link)
	if link.ID == "" || link.FilePath != filepath.Join("docs", "report.txt") || link.ExpiresAt != 0 {
		t.Fatalf("preview link = %#v", link)
	}

	if _, err := os.Stat(rt.PreviewLinks.filePath); err != nil {
		t.Fatalf("expected preview links to be persisted: %v", err)
	}

	resp = performRequest(router, http.MethodGet, "/preview-links", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list preview links status = %d: %s", resp.Code, resp.Body.String())
	}
	var links []PreviewLink
	decodeRawData(t, resp, &links)
	if len(links) != 1 || links[0].ID != link.ID {
		t.Fatalf("listed preview links = %#v", links)
	}

	resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/info", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("preview info status = %d: %s", resp.Code, resp.Body.String())
	}
	var info map[string]any
	decodeRawData(t, resp, &info)
	if info["file_name"] != "report.txt" || info["require_password"] != false || info["previewable"] != true {
		t.Fatalf("preview info = %#v", info)
	}

	resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/file", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("preview file status = %d: %s", resp.Code, resp.Body.String())
	}
	if strings.TrimSpace(resp.Body.String()) != "report body" {
		t.Fatalf("preview file body = %q", resp.Body.String())
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("content disposition = %q", got)
	}

	resp = performRequest(router, http.MethodDelete, "/preview/"+link.ID, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete preview link status = %d: %s", resp.Code, resp.Body.String())
	}
	resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/info", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("deleted preview info status = %d, want 404", resp.Code)
	}
}

func TestPreviewFilePasswordAndExpiry(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*previewLinkEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte("secret body"), 0644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	rt.PreviewLinks.Lock()
	rt.PreviewLinks.m["protected"] = &previewLinkEntry{
		FilePaths:    []string{"secret.txt"},
		PasswordHash: hashPassword("secret"),
		CreatedAt:    time.Now(),
	}
	rt.PreviewLinks.m["expired"] = &previewLinkEntry{
		FilePaths: []string{"secret.txt"},
		ExpiresAt: time.Now().Add(-time.Minute),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	rt.PreviewLinks.Unlock()

	router := gin.New()
	router.GET("/preview/:linkId/info", rt.PreviewInfoHandler())
	router.GET("/preview/:linkId/file", rt.PreviewFileHandler())

	resp := performRequest(router, http.MethodGet, "/preview/protected/info", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("protected info status = %d: %s", resp.Code, resp.Body.String())
	}
	var info map[string]any
	decodeRawData(t, resp, &info)
	if info["require_password"] != true || info["file_name"] != "secret.txt" {
		t.Fatalf("protected info = %#v", info)
	}

	resp = performRequest(router, http.MethodGet, "/preview/protected/file", nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("missing password status = %d, want 401", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/preview/protected/file?password=bad", nil)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("wrong password status = %d, want 403", resp.Code)
	}

	reqResp := performRequestWithHeader(router, http.MethodGet, "/preview/protected/file", "X-Preview-Password", "secret")
	if reqResp.Code != http.StatusOK {
		t.Fatalf("correct password status = %d: %s", reqResp.Code, reqResp.Body.String())
	}
	if strings.TrimSpace(reqResp.Body.String()) != "secret body" {
		t.Fatalf("protected file body = %q", reqResp.Body.String())
	}

	resp = performRequest(router, http.MethodGet, "/preview/expired/info", nil)
	if resp.Code != http.StatusGone {
		t.Fatalf("expired info status = %d, want 410", resp.Code)
	}
	rt.PreviewLinks.RLock()
	_, exists := rt.PreviewLinks.m["expired"]
	rt.PreviewLinks.RUnlock()
	if exists {
		t.Fatal("expired preview link should be removed")
	}
}

func TestPreviewMarkdownDoesNotDownloadUnlessRequested(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*previewLinkEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.WriteFile(filepath.Join(workspace, "note.md"), []byte("# note\n"), 0644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	rt.PreviewLinks.Lock()
	rt.PreviewLinks.m["markdown"] = &previewLinkEntry{
		FilePaths: []string{"note.md"},
		CreatedAt: time.Now(),
	}
	rt.PreviewLinks.Unlock()

	router := gin.New()
	router.GET("/preview/:linkId/file", rt.PreviewFileHandler())
	router.GET("/preview/:linkId/render/*filepath", rt.PreviewRenderHandler())

	resp := performRequest(router, http.MethodGet, "/preview/markdown/file", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("preview markdown status = %d: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("preview markdown disposition = %q, want inline", got)
	}

	resp = performRequest(router, http.MethodGet, "/preview/markdown/file?download=1", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("download markdown status = %d: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("download markdown disposition = %q, want attachment", got)
	}

	resp = performRequest(router, http.MethodGet, "/preview/markdown/render/note.md", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("render markdown status = %d: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("render markdown disposition = %q, want inline", got)
	}
}

func TestPreviewFileMultiFileZip(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*previewLinkEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	rt.PreviewLinks.Lock()
	rt.PreviewLinks.m["multi"] = &previewLinkEntry{
		FilePaths: []string{"a.txt", "b.txt"},
		CreatedAt: time.Now(),
	}
	rt.PreviewLinks.Unlock()

	router := gin.New()
	router.GET("/preview/:linkId/file", rt.PreviewFileHandler())

	resp := performRequest(router, http.MethodGet, "/preview/multi/file", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("multi preview status = %d: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type = %q, want application/zip", got)
	}
	reader, err := zip.NewReader(bytes.NewReader(resp.Body.Bytes()), int64(resp.Body.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	names := make(map[string]bool)
	for _, file := range reader.File {
		names[file.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Fatalf("zip names = %#v", names)
	}
}

func TestCreatePreviewLinkRejectsInvalidRequests(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*previewLinkEntry{})
	gin.SetMode(gin.TestMode)
	if err := os.WriteFile(filepath.Join(workspace, "exists.txt"), []byte("ok"), 0644); err != nil {
		t.Fatalf("write exists: %v", err)
	}

	router := gin.New()
	router.POST("/preview-links", rt.CreatePreviewLinkHandler())
	router.DELETE("/preview/:linkId", rt.DeletePreviewLinkHandler())

	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "bad json", body: `{bad json`, want: http.StatusBadRequest},
		{name: "missing path", body: `{}`, want: http.StatusBadRequest},
		{name: "traversal", body: `{"file_path":"../outside.txt"}`, want: http.StatusBadRequest},
		{name: "missing file", body: `{"file_path":"missing.txt"}`, want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSON(router, http.MethodPost, "/preview-links", tt.body)
			if resp.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", resp.Code, tt.want, resp.Body.String())
			}
		})
	}

	resp := performRequest(router, http.MethodDelete, "/preview/missing", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want 404", resp.Code)
	}
}

func performRequestWithHeader(router http.Handler, method, path, header, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(header, value)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

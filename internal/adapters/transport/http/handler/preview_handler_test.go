package handler

import (
	"archive/zip"
	"bytes"
	sharestore "fkteams/internal/adapters/storage/file/share"
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
	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
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

	if _, err := os.Stat(rt.PreviewLinks.Path()); err != nil {
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
	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte("secret body"), 0644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := rt.PreviewLinks.Put("protected", &sharestore.PreviewEntry{
		FilePaths:    []string{"secret.txt"},
		PasswordHash: hashPassword("secret"),
		CreatedAt:    time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.PreviewLinks.Put("expired", &sharestore.PreviewEntry{
		FilePaths: []string{"secret.txt"},
		ExpiresAt: time.Now().Add(-time.Minute),
		CreatedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/preview/:linkId/info", rt.PreviewInfoHandler())
	router.GET("/preview/:linkId/file", rt.PreviewFileHandler())
	router.POST("/preview/:linkId/auth", rt.PreviewAuthHandler())
	router.GET("/preview/:linkId/render/*filepath", rt.PreviewRenderHandler())

	resp := performRequest(router, http.MethodGet, "/preview/protected/info", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("protected info status = %d: %s", resp.Code, resp.Body.String())
	}
	var info map[string]any
	decodeRawData(t, resp, &info)
	if info["require_password"] != true || info["authorized"] != false || info["file_name"] != "secret.txt" {
		t.Fatalf("protected info = %#v", info)
	}

	resp = performRequest(router, http.MethodGet, "/preview/protected/file", nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("missing password status = %d, want 401", resp.Code)
	}
	resp = performRequest(router, http.MethodGet, "/preview/protected/file?password=secret", nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("query password status = %d, want 401", resp.Code)
	}

	resp = performJSON(router, http.MethodPost, "/preview/protected/auth", `{"password":"bad"}`)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", resp.Code)
	}

	resp = performJSON(router, http.MethodPost, "/preview/protected/auth", `{"password":"secret"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("preview auth status = %d: %s", resp.Code, resp.Body.String())
	}
	cookies := resp.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("preview auth cookies = %#v", cookies)
	}
	grantCookie := cookies[0]
	if grantCookie.Name != previewGrantCookieName("protected") || strings.Contains(grantCookie.Value, "secret") {
		t.Fatalf("preview grant exposes password or uses wrong name: %#v", grantCookie)
	}
	if grantCookie.Path != "/api/fkteams/preview/protected" || !grantCookie.HttpOnly || grantCookie.Secure || grantCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("insecure preview grant cookie: %#v", grantCookie)
	}
	if grantCookie.MaxAge != int(previewGrantTTL/time.Second) {
		t.Fatalf("preview grant MaxAge = %d, want %d", grantCookie.MaxAge, int(previewGrantTTL/time.Second))
	}

	infoRequest := httptest.NewRequest(http.MethodGet, "/preview/protected/info", nil)
	infoRequest.AddCookie(grantCookie)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, infoRequest)
	if resp.Code != http.StatusOK {
		t.Fatalf("authorized info status = %d: %s", resp.Code, resp.Body.String())
	}
	info = nil
	decodeRawData(t, resp, &info)
	if info["authorized"] != true {
		t.Fatalf("authorized preview info = %#v", info)
	}

	fileRequest := httptest.NewRequest(http.MethodGet, "/preview/protected/file", nil)
	fileRequest.AddCookie(grantCookie)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, fileRequest)
	if resp.Code != http.StatusOK {
		t.Fatalf("authorized file status = %d: %s", resp.Code, resp.Body.String())
	}
	if strings.TrimSpace(resp.Body.String()) != "secret body" {
		t.Fatalf("protected file body = %q", resp.Body.String())
	}

	renderRequest := httptest.NewRequest(http.MethodGet, "/preview/protected/render/?password=secret", nil)
	renderRequest.AddCookie(grantCookie)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, renderRequest)
	if resp.Code != http.StatusFound {
		t.Fatalf("render redirect status = %d: %s", resp.Code, resp.Body.String())
	}
	if location := resp.Header().Get("Location"); strings.Contains(location, "?") || strings.Contains(location, "password=") {
		t.Fatalf("render redirect leaked password: %q", location)
	}

	resp = performRequest(router, http.MethodGet, "/preview/expired/info", nil)
	if resp.Code != http.StatusGone {
		t.Fatalf("expired info status = %d, want 410", resp.Code)
	}
	_, exists := rt.PreviewLinks.Lookup("expired")
	if exists {
		t.Fatal("expired preview link should be removed")
	}
}

func TestPreviewGrantValidation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	grant := generatePreviewGrant("link-1", "password-hash", now.Add(time.Hour))

	if !validatePreviewGrant(grant, "link-1", "password-hash", now) {
		t.Fatal("expected preview grant to be valid")
	}
	if validatePreviewGrant(grant, "link-2", "password-hash", now) {
		t.Fatal("preview grant must be bound to its link")
	}
	if validatePreviewGrant(grant, "link-1", "changed-password-hash", now) {
		t.Fatal("password change must invalidate preview grant")
	}
	if validatePreviewGrant(grant, "link-1", "password-hash", now.Add(time.Hour)) {
		t.Fatal("expired preview grant must be rejected")
	}
	if validatePreviewGrant("malformed", "link-1", "password-hash", now) {
		t.Fatal("malformed preview grant must be rejected")
	}
}

func TestPreviewAuthRateLimitsInvalidPasswords(t *testing.T) {
	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{
		"rate-limited": {
			FilePaths:    []string{"secret.txt"},
			PasswordHash: hashPassword("secret"),
			CreatedAt:    time.Now(),
		},
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/preview/:linkId/auth", rt.PreviewAuthHandler())

	const remoteIP = "198.51.100.31"
	attemptKey := "preview:rate-limited:" + remoteIP
	publicShareAttempts.Reset(attemptKey)
	t.Cleanup(func() { publicShareAttempts.Reset(attemptKey) })

	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/preview/rate-limited/auth", strings.NewReader(`{"password":"bad"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remoteIP + ":1234"
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401: %s", i+1, resp.Code, resp.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/preview/rate-limited/auth", strings.NewReader(`{"password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteIP + ":1234"
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusTooManyRequests || resp.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limited status = %d, Retry-After=%q: %s", resp.Code, resp.Header().Get("Retry-After"), resp.Body.String())
	}
}

func TestPreviewMarkdownDoesNotDownloadUnlessRequested(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.WriteFile(filepath.Join(workspace, "note.md"), []byte("# note\n"), 0644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := rt.PreviewLinks.Put("markdown", &sharestore.PreviewEntry{
		FilePaths: []string{"note.md"},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

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
	assertUntrustedContentHeaders(t, resp)

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
	assertUntrustedContentHeaders(t, resp)
}

func TestPreviewFileMultiFileZip(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
	gin.SetMode(gin.TestMode)

	if err := os.WriteFile(filepath.Join(workspace, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := rt.PreviewLinks.Put("multi", &sharestore.PreviewEntry{
		FilePaths: []string{"a.txt", "b.txt"},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

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
	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
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

func TestPreviewRenderUsesExplicitResourceManifest(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	siteDir := filepath.Join(workspace, "site")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		t.Fatalf("mkdir site: %v", err)
	}
	for name, content := range map[string]string{
		"index.html": "<link rel=\"stylesheet\" href=\"style.css\"><h1>site</h1>",
		"style.css":  "body { color: black; }",
		"secret.txt": "private",
	} {
		if err := os.WriteFile(filepath.Join(siteDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/preview-links", rt.CreatePreviewLinkHandler())
	router.GET("/preview/:linkId/render/*filepath", rt.PreviewRenderHandler())

	resp := performJSON(router, http.MethodPost, "/preview-links", `{"file_paths":["site/index.html","site/style.css"]}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("create preview link status = %d: %s", resp.Code, resp.Body.String())
	}
	var link PreviewLink
	decodeRawData(t, resp, &link)

	for _, resource := range []string{"index.html", "style.css"} {
		resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/render/"+resource, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("render %s status = %d: %s", resource, resp.Code, resp.Body.String())
		}
	}
	resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/render/secret.txt", nil)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("unshared sibling status = %d, want 403: %s", resp.Code, resp.Body.String())
	}

	outsideFile := filepath.Join(t.TempDir(), "outside.css")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Remove(filepath.Join(siteDir, "style.css")); err != nil {
		t.Fatalf("remove style: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(siteDir, "style.css")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/render/style.css", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("replaced symlink status = %d, want 404: %s", resp.Code, resp.Body.String())
	}
	resp = performJSON(router, http.MethodPost, "/preview-links", `{"file_path":"site/style.css"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("share symlink status = %d, want 400: %s", resp.Code, resp.Body.String())
	}
}

func TestPreviewDirectoryDownloadUsesCreationManifest(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	docsDir := filepath.Join(workspace, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "existing.txt"), []byte("existing"), 0644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	rt := newPreviewTestRuntime(t, map[string]*sharestore.PreviewEntry{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/preview-links", rt.CreatePreviewLinkHandler())
	router.GET("/preview/:linkId/file", rt.PreviewFileHandler())

	resp := performJSON(router, http.MethodPost, "/preview-links", `{"file_path":"docs"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("create directory preview status = %d: %s", resp.Code, resp.Body.String())
	}
	var link PreviewLink
	decodeRawData(t, resp, &link)
	if err := os.WriteFile(filepath.Join(docsDir, "late.txt"), []byte("late"), 0644); err != nil {
		t.Fatalf("write late file: %v", err)
	}

	resp = performRequest(router, http.MethodGet, "/preview/"+link.ID+"/file", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("download directory preview status = %d: %s", resp.Code, resp.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(resp.Body.Bytes()), int64(resp.Body.Len()))
	if err != nil {
		t.Fatalf("read directory zip: %v", err)
	}
	names := make(map[string]bool)
	for _, file := range reader.File {
		names[file.Name] = true
	}
	if !names["docs/existing.txt"] || names["docs/late.txt"] {
		t.Fatalf("directory manifest zip names = %#v", names)
	}
}

func performRequestWithHeader(router http.Handler, method, path, header, value string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(header, value)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

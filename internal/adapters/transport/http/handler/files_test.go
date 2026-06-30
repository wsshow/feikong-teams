package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fkteams/internal/app/appdata"
	"fkteams/internal/runtime/env"

	"github.com/gin-gonic/gin"
)

type rawResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func TestResolveAndValidatePathBoundaries(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		t.Fatalf("abs base: %v", err)
	}

	fullPath, relPath, err := resolveAndValidatePath(baseDir, absBase, "dir/file.txt")
	if err != nil {
		t.Fatalf("resolve valid path: %v", err)
	}
	if !strings.HasPrefix(fullPath, absBase) || relPath != filepath.Join("dir", "file.txt") {
		t.Fatalf("unexpected resolved path full=%q rel=%q", fullPath, relPath)
	}
	if _, _, err := resolveAndValidatePath(baseDir, absBase, "../outside.txt"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
	if isPathWithinBase(absBase, absBase+"-sibling/file.txt") {
		t.Fatal("expected sibling prefix path to be outside base")
	}
	if !isPathWithinBase(absBase, filepath.Join(absBase, "file.txt")) {
		t.Fatal("expected child path to be inside base")
	}
}

func TestGetFilesAndSearchHandlers(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	if err := os.Mkdir(filepath.Join(workspace, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "docs", "guide"), 0755); err != nil {
		t.Fatalf("mkdir docs guide: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("note"), 0644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "docs", "guide", "intro.md"), []byte("intro"), 0644); err != nil {
		t.Fatalf("write nested intro: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".secret"), []byte("secret"), 0644); err != nil {
		t.Fatalf("write hidden: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/files", GetFilesHandler())
	router.GET("/search", SearchFilesHandler())

	resp := performRequest(router, http.MethodGet, "/files", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("list files status = %d: %s", resp.Code, resp.Body.String())
	}
	var files []FileInfo
	decodeRawData(t, resp, &files)
	if len(files) != 2 {
		t.Fatalf("expected visible dir and file, got %#v", files)
	}
	if files[0].Name != "docs" || !files[0].IsDir {
		t.Fatalf("expected directories first, got %#v", files)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name, ".") {
			t.Fatalf("hidden file should be omitted: %#v", file)
		}
	}

	resp = performRequest(router, http.MethodGet, "/search?q=NOTE", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("search status = %d: %s", resp.Code, resp.Body.String())
	}
	var results []FileInfo
	decodeRawData(t, resp, &results)
	if len(results) != 1 || results[0].Name != "note.txt" {
		t.Fatalf("unexpected search results: %#v", results)
	}

	resp = performRequest(router, http.MethodGet, "/search?q=docs/guide", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("path search status = %d: %s", resp.Code, resp.Body.String())
	}
	decodeRawData(t, resp, &results)
	if len(results) != 2 || results[0].Path != "docs/guide" || results[1].Path != "docs/guide/intro.md" {
		t.Fatalf("unexpected path search results: %#v", results)
	}

	resp = performRequest(router, http.MethodGet, "/search?q=", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("empty search status = %d, want 400", resp.Code)
	}
}

func TestDeleteFileHandler(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	filePath := filepath.Join(workspace, "delete-me.txt")
	if err := os.WriteFile(filePath, []byte("delete"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	nonEmptyDir := filepath.Join(workspace, "non-empty")
	if err := os.Mkdir(nonEmptyDir, 0755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "child.txt"), []byte("child"), 0644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/delete", DeleteFileHandler())

	resp := performJSON(router, http.MethodPost, "/delete", `{"path":"."}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("root delete status = %d, want 400", resp.Code)
	}

	resp = performJSON(router, http.MethodPost, "/delete", `{"path":"non-empty"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("non-empty delete status = %d, want 400", resp.Code)
	}
	if _, err := os.Stat(nonEmptyDir); err != nil {
		t.Fatalf("non-empty dir should still exist: %v", err)
	}

	resp = performJSON(router, http.MethodPost, "/delete", `{"path":"non-empty","force":true}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("force delete status = %d: %s", resp.Code, resp.Body.String())
	}
	if _, err := os.Stat(nonEmptyDir); !os.IsNotExist(err) {
		t.Fatalf("expected force-deleted dir to be gone, stat err=%v", err)
	}

	resp = performJSON(router, http.MethodPost, "/delete", `{"path":"delete-me.txt"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("file delete status = %d: %s", resp.Code, resp.Body.String())
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected deleted file to be gone, stat err=%v", err)
	}
}

func TestFileContentHandlers(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	filePath := filepath.Join(workspace, "note.md")
	if err := os.WriteFile(filePath, []byte("# title\n"), 0644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/content", GetFileContentHandler())
	router.PUT("/content", SaveFileContentHandler())

	resp := performRequest(router, http.MethodGet, "/content?path=note.md", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("get content status = %d: %s", resp.Code, resp.Body.String())
	}
	var got struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	decodeRawData(t, resp, &got)
	if got.Path != "note.md" || got.Content != "# title\n" {
		t.Fatalf("unexpected content response: %#v", got)
	}

	resp = performJSON(router, http.MethodPut, "/content", `{"path":"note.md","content":"updated\n"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("save content status = %d: %s", resp.Code, resp.Body.String())
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "updated\n" {
		t.Fatalf("unexpected saved content: %q", data)
	}

	resp = performRequest(router, http.MethodGet, "/content?path=docs", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("directory content status = %d, want 400", resp.Code)
	}
}

func TestServeFileHandler(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	siteDir := filepath.Join(workspace, "site")
	if err := os.Mkdir(siteDir, 0755); err != nil {
		t.Fatalf("mkdir site: %v", err)
	}
	if err := os.WriteFile(filepath.Join(siteDir, "index.html"), []byte("<h1>hello</h1>"), 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/view/*filepath", ServeFileHandler())

	resp := performRequest(router, http.MethodGet, "/view/site", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("serve index status = %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "hello") {
		t.Fatalf("expected served index content, got %q", resp.Body.String())
	}

	resp = performRequest(router, http.MethodGet, "/view/../outside.txt", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("traversal serve status = %d, want 400", resp.Code)
	}
}

func TestSanitizeUploadID(t *testing.T) {
	got := sanitizeUploadID("../upload-id")
	if len(got) != 32 {
		t.Fatalf("sanitizeUploadID length = %d, want 32", len(got))
	}
	if strings.ContainsAny(got, `/\.`) {
		t.Fatalf("sanitizeUploadID should only contain hex chars, got %q", got)
	}
	if got != sanitizeUploadID("../upload-id") {
		t.Fatal("sanitizeUploadID should be stable")
	}
}

func setupWorkspaceDir(t *testing.T) string {
	t.Helper()

	t.Setenv(env.AppDir, t.TempDir())
	workspace := appdata.WorkspaceDir()
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	return workspace
}

func performRequest(router http.Handler, method, path string, body *strings.Reader) *httptest.ResponseRecorder {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, body)
	}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func decodeRawData(t *testing.T, resp *httptest.ResponseRecorder, target any) {
	t.Helper()

	var envelope rawResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if envelope.Code != 0 {
		t.Fatalf("unexpected response envelope: %#v", envelope)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("unmarshal response data: %v", err)
	}
}

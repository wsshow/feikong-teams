package handler

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadFileHandlerWritesNestedFileInWorkspace(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", UploadFileHandler())

	response := performMultipartFileUpload(t, router, "/upload", "nested", `C:\fakepath\note.txt`, []byte("content"))
	if response.Code != http.StatusOK {
		t.Fatalf("upload status = %d: %s", response.Code, response.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(workspace, "nested", "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "content" {
		t.Fatalf("uploaded content = %q", content)
	}
}

func TestUploadFileHandlerRejectsSymlinkDirectory(t *testing.T) {
	workspace := setupWorkspaceDir(t)
	realDirectory := filepath.Join(workspace, "real")
	if err := os.Mkdir(realDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDirectory, filepath.Join(workspace, "linked")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", UploadFileHandler())
	response := performMultipartFileUpload(t, router, "/upload", "linked", "note.txt", []byte("content"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("upload status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(realDirectory, "note.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file should not be written through symlink, stat error = %v", err)
	}
}

func TestAssembleChunkUploadIsAtomic(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	chunks := t.TempDir()
	if err := os.WriteFile(filepath.Join(chunks, "0"), []byte("hello "), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	if _, err := assembleChunkUpload(root, "target.txt", chunks, 2); err == nil {
		t.Fatal("assembleChunkUpload() should fail when a chunk is missing")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("target changed after failed assembly: %q", content)
	}

	if err := os.WriteFile(filepath.Join(chunks, "1"), []byte("world"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := assembleChunkUpload(root, "target.txt", chunks, 2); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello world" {
		t.Fatalf("assembled content = %q", content)
	}
}

func performMultipartFileUpload(t *testing.T, router http.Handler, endpoint, path, fileName string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("path", path); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, endpoint, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

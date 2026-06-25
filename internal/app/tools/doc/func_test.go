package doc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDocumentInfoForTextFile(t *testing.T) {
	path := writeDocTestFile(t, "note.txt", "hello\nworld\n")

	resp, err := GetDocumentInfo(context.Background(), &GetDocumentInfoRequest{FilePath: path})
	if err != nil {
		t.Fatalf("GetDocumentInfo() error = %v", err)
	}
	if resp.ErrorMessage != "" {
		t.Fatalf("GetDocumentInfo() error message = %q", resp.ErrorMessage)
	}
	if resp.FilePath != path || resp.FileType != "TXT" {
		t.Fatalf("response = %#v, want TXT file path", resp)
	}
	if resp.FileSize == "" || resp.EstimatedSize == "" {
		t.Fatalf("response sizes missing: %#v", resp)
	}
}

func TestDocumentReadFunctionsReportMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.txt")

	info, err := GetDocumentInfo(context.Background(), &GetDocumentInfoRequest{FilePath: missing})
	if err != nil {
		t.Fatalf("GetDocumentInfo() error = %v", err)
	}
	if !strings.Contains(info.ErrorMessage, "文件访问失败") {
		t.Fatalf("info error = %q, want file access error", info.ErrorMessage)
	}

	pages, err := ReadDocumentByPages(context.Background(), &ReadDocumentByPagesRequest{FilePath: missing, EndPage: -1})
	if err != nil {
		t.Fatalf("ReadDocumentByPages() error = %v", err)
	}
	if !strings.Contains(pages.ErrorMessage, "读取文档失败") {
		t.Fatalf("pages error = %q, want read error", pages.ErrorMessage)
	}

	lines, err := ReadDocumentByLines(context.Background(), &ReadDocumentByLinesRequest{FilePath: missing, EndLine: -1, PageIndex: -1})
	if err != nil {
		t.Fatalf("ReadDocumentByLines() error = %v", err)
	}
	if !strings.Contains(lines.ErrorMessage, "读取文档失败") {
		t.Fatalf("lines error = %q, want read error", lines.ErrorMessage)
	}

	smart, err := ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{FilePath: missing})
	if err != nil {
		t.Fatalf("ReadDocumentSmart() error = %v", err)
	}
	if !strings.Contains(smart.ErrorMessage, "读取文档失败") {
		t.Fatalf("smart error = %q, want read error", smart.ErrorMessage)
	}
}

func TestReadDocumentSmartReturnsFullContentWhenSmall(t *testing.T) {
	path := writeDocTestFile(t, "small.txt", "alpha\nbeta")

	resp, err := ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{
		FilePath:     path,
		MaxChars:     100,
		CleanContent: false,
	})
	if err != nil {
		t.Fatalf("ReadDocumentSmart() error = %v", err)
	}
	if resp.ErrorMessage != "" {
		t.Fatalf("ReadDocumentSmart() error message = %q", resp.ErrorMessage)
	}
	if resp.IsTruncated {
		t.Fatal("small document should not be truncated")
	}
	if resp.Strategy != "完整读取" {
		t.Fatalf("strategy = %q, want 完整读取", resp.Strategy)
	}
	if !strings.Contains(resp.Content, "alpha") || !strings.Contains(resp.Content, "beta") {
		t.Fatalf("content = %q, want source text", resp.Content)
	}
	if resp.ReturnedSize != len(resp.Content) || resp.OriginalSize != len(resp.Content) {
		t.Fatalf("sizes = returned %d original %d content %d", resp.ReturnedSize, resp.OriginalSize, len(resp.Content))
	}
}

func TestReadDocumentSmartTruncatesLargeContent(t *testing.T) {
	path := writeDocTestFile(t, "large.txt", strings.Repeat("0123456789", 100))

	resp, err := ReadDocumentSmart(context.Background(), &ReadDocumentSmartRequest{
		FilePath:     path,
		MaxChars:     120,
		CleanContent: false,
	})
	if err != nil {
		t.Fatalf("ReadDocumentSmart() error = %v", err)
	}
	if resp.ErrorMessage != "" {
		t.Fatalf("ReadDocumentSmart() error message = %q", resp.ErrorMessage)
	}
	if !resp.IsTruncated {
		t.Fatal("large document should be truncated")
	}
	if resp.Strategy != "从头截断" {
		t.Fatalf("strategy = %q, want 从头截断", resp.Strategy)
	}
	if len(resp.Content) != 120 || resp.ReturnedSize != 120 {
		t.Fatalf("returned length = %d size = %d, want 120", len(resp.Content), resp.ReturnedSize)
	}
	if resp.Suggestion == "" {
		t.Fatal("truncated response should include suggestion")
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{size: 512, want: "512 B"},
		{size: 1536, want: "1.50 KB"},
		{size: 2 * 1024 * 1024, want: "2.00 MB"},
		{size: 3 * 1024 * 1024 * 1024, want: "3.00 GB"},
	}
	for _, tt := range tests {
		if got := formatFileSize(tt.size); got != tt.want {
			t.Fatalf("formatFileSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestSampleContent(t *testing.T) {
	content := strings.Repeat("a", 200) + strings.Repeat("b", 200) + strings.Repeat("c", 200)

	if got := sampleContent("short", 100); got != "short" {
		t.Fatalf("sampleContent(short) = %q", got)
	}
	sampled := sampleContent(content, 400)
	if len(sampled) > 400 {
		t.Fatalf("sampled length = %d, want <= 400", len(sampled))
	}
	if !strings.Contains(sampled, "... [中间部分] ...") || !strings.Contains(sampled, "... [后续部分] ...") {
		t.Fatalf("sampled content missing separators: %q", sampled)
	}
	if got := sampleContent(content, 100); got != content[:100] {
		t.Fatalf("small max sample = %q, want direct prefix", got)
	}
}

func TestGetTools(t *testing.T) {
	tools, err := GetTools()
	if err != nil {
		t.Fatalf("GetTools() error = %v", err)
	}
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info: %v", err)
		}
		got = append(got, info.Name)
	}
	want := []string{"get_document_info", "read_document_smart", "read_document_by_pages", "read_document_by_lines"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func writeDocTestFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test document: %v", err)
	}
	return path
}

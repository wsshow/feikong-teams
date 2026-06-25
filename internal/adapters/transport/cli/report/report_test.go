package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertMarkdownToHTML(t *testing.T) {
	html := string(ConvertMarkdownToHTML([]byte("# 标题\n\n[链接](https://example.com)\n")))

	for _, want := range []string{
		`<h1 id="标题">标题</h1>`,
		`href="https://example.com"`,
		`target="_blank"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html = %q, missing %q", html, want)
		}
	}
}

func TestConvertMarkdownFileToHTML(t *testing.T) {
	mdPath := writeMarkdown(t, "report.md", "## 小结\n\n内容")

	html, err := ConvertMarkdownFileToHTML(mdPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownFileToHTML() error = %v", err)
	}
	if !strings.Contains(string(html), "<h2") || !strings.Contains(string(html), "小结") {
		t.Fatalf("html = %q, want heading content", html)
	}
}

func TestConvertMarkdownFileToHTMLFile(t *testing.T) {
	mdPath := writeMarkdown(t, "plain.md", "# Plain")

	htmlPath, err := ConvertMarkdownFileToHTMLFile(mdPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownFileToHTMLFile() error = %v", err)
	}
	if htmlPath != strings.TrimSuffix(mdPath, ".md")+".html" {
		t.Fatalf("html path = %q", htmlPath)
	}
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html file: %v", err)
	}
	if !strings.Contains(string(data), "Plain") {
		t.Fatalf("html file = %q, want Plain", data)
	}
}

func TestConvertMarkdownFileToNiceHTMLFile(t *testing.T) {
	mdPath := writeMarkdown(t, "nice.md", "# Nice")

	htmlPath, err := ConvertMarkdownFileToNiceHTMLFile(mdPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownFileToNiceHTMLFile() error = %v", err)
	}
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read nice html file: %v", err)
	}
	html := string(data)
	for _, want := range []string{"<!DOCTYPE html>", "非空小队 - 历史记录", "markdown-body", "Nice"} {
		if !strings.Contains(html, want) {
			t.Fatalf("nice html = %q, missing %q", html, want)
		}
	}
}

func TestConvertMarkdownFileReportsReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.md")

	if _, err := ConvertMarkdownFileToHTML(missing); err == nil {
		t.Fatal("ConvertMarkdownFileToHTML() error = nil, want read error")
	}
	if _, err := ConvertMarkdownFileToHTMLFile(missing); err == nil {
		t.Fatal("ConvertMarkdownFileToHTMLFile() error = nil, want read error")
	}
	if _, err := ConvertMarkdownFileToNiceHTMLFile(missing); err == nil {
		t.Fatal("ConvertMarkdownFileToNiceHTMLFile() error = nil, want read error")
	}
}

func writeMarkdown(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write markdown file: %v", err)
	}
	return path
}

package eventview

import (
	"strings"
	"testing"
)

func TestUnclosedFenceRange(t *testing.T) {
	content := "说明\n\n```go\npackage main\n"
	start, bodyStart, inFence := unclosedFenceRange(content)

	if !inFence {
		t.Fatal("expected unclosed fence")
	}
	if content[start:bodyStart] != "```go\n" {
		t.Fatalf("opening fence range = %q", content[start:bodyStart])
	}
}

func TestResponseStatusViewShowsOnlyCodeProgress(t *testing.T) {
	out := responseStatusView("package main\n", 120)

	if sameStreamMessage("代码块未闭合", out) {
		t.Fatalf("status should not mention fence state: %q", out)
	}
	if !sameStreamMessage("代码生成中", out) || !sameStreamMessage("12 字", out) || !sameStreamMessage("1 行", out) {
		t.Fatalf("status should show only progress: %q", out)
	}
	if !strings.Contains(out, "> **代码生成中**") {
		t.Fatalf("status should use markdown styling: %q", out)
	}
}

func TestCodeProgressContentExcludesFenceMarkers(t *testing.T) {
	content := "```go\npackage main\nfunc main() {}\n```"
	got := codeProgressContent(content)
	want := "package main\nfunc main() {}"

	if got != want {
		t.Fatalf("codeProgressContent() = %q, want %q", got, want)
	}
}

func TestStreamPreviewMarkdownFoldsOnlyCodeBlocks(t *testing.T) {
	content := "## 标题\n\n正文 **加粗**\n\n```go\npackage main\nfunc main() {}\n```\n\n结尾"
	got := streamPreviewMarkdown(content)

	if !strings.Contains(got, "## 标题") || !strings.Contains(got, "正文 **加粗**") || !strings.Contains(got, "结尾") {
		t.Fatalf("preview should preserve normal markdown: %q", got)
	}
	if strings.Contains(got, "package main") || strings.Contains(got, "func main") || strings.Contains(got, "```") {
		t.Fatalf("preview should fold code block content and fences: %q", got)
	}
	if !strings.Contains(got, "代码生成中") {
		t.Fatalf("preview should include code progress: %q", got)
	}
}

func TestStreamPreviewMarkdownFoldsMultipleCodeBlocks(t *testing.T) {
	content := "前言\n\n```go\npackage main\n```\n\n中间\n\n~~~json\n{\"ok\": true}\n~~~\n\n结尾"
	got := streamPreviewMarkdown(content)

	if !strings.Contains(got, "前言") || !strings.Contains(got, "中间") || !strings.Contains(got, "结尾") {
		t.Fatalf("preview should preserve surrounding text: %q", got)
	}
	if strings.Contains(got, "package main") || strings.Contains(got, "{\"ok\": true}") {
		t.Fatalf("preview should fold every code block: %q", got)
	}
	if count := strings.Count(got, "代码生成中"); count != 2 {
		t.Fatalf("preview should show two code progress lines, got %d in %q", count, got)
	}
}

func TestTruncateVisibleUsesDisplayWidth(t *testing.T) {
	out := truncateVisible("你好世界abcdef", 8)

	if out != "你好..." {
		t.Fatalf("truncateVisible() = %q, want %q", out, "你好...")
	}
}

func TestSameStreamMessageDetectsDuplicateFinalMessage(t *testing.T) {
	if !sameStreamMessage("package main\n", "package main") {
		t.Fatal("expected equal stream and final message to match")
	}
	if !sameStreamMessage("package main", "package main\nfunc main() {}") {
		t.Fatal("expected final message containing stream content to match")
	}
	if sameStreamMessage("package main\nfunc main() {}", "package main") {
		t.Fatal("shorter final message should not discard longer stream content")
	}
	if sameStreamMessage("alpha", "beta") {
		t.Fatal("unrelated messages should not match")
	}
}

func TestFenceLineDetection(t *testing.T) {
	fence, ok := openingFence("```go\n")
	if !ok || fence != "```" {
		t.Fatalf("openingFence() = %q, %v; want ``` true", fence, ok)
	}
	if !isClosingFenceLine("```\n", fence) {
		t.Fatal("expected closing fence")
	}
	if isClosingFenceLine("fmt.Println(\"hi\")\n", fence) {
		t.Fatal("ordinary code line should not close fence")
	}
}

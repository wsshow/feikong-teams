package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownTableUsesClosedBorder(t *testing.T) {
	out := RenderMarkdown("| 项目 | 状态 |\n| --- | --- |\n| 表格 | 完成 |")

	for _, token := range []string{"┌", "┐", "└", "┘", "│", "├", "┤"} {
		if !strings.Contains(out, token) {
			t.Fatalf("expected closed table border token %q in output:\n%s", token, out)
		}
	}
}

func TestRenderMarkdownWithWidthTableUsesClosedBorder(t *testing.T) {
	out := RenderMarkdownWithWidth("| 项目 | 状态 |\n| --- | --- |\n| 表格 | 完成 |", 32)

	for _, token := range []string{"┌", "┐", "└", "┘", "│", "├", "┼", "┤"} {
		if !strings.Contains(out, token) {
			t.Fatalf("expected closed table border token %q in output:\n%s", token, out)
		}
	}
}

func TestRenderMarkdownTableSeparatesBodyRows(t *testing.T) {
	out := RenderMarkdownWithWidth("| ID | 姓名 |\n| --- | --- |\n| 001 | 张三 |\n| 002 | 李四 |", 40)

	if strings.Count(out, "├") < 2 || strings.Count(out, "┼") < 2 || strings.Count(out, "┤") < 2 {
		t.Fatalf("expected table body rows to include horizontal separators:\n%s", out)
	}
}

func TestRenderMarkdownTableRendersInlineMarkdown(t *testing.T) {
	out := RenderMarkdownWithWidth("| 包 | 路径 | 职责 |\n| --- | --- | --- |\n| **pi-ai** | `packages/ai` | **核心** CLI |", 80)
	plain := StripANSI(out)

	if strings.Contains(plain, "**pi-ai**") || strings.Contains(plain, "`packages/ai`") || strings.Contains(plain, "**核心**") {
		t.Fatalf("expected table cells to render inline markdown:\n%s", out)
	}
	for _, want := range []string{"pi-ai", "packages/ai", "核心", "CLI"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected rendered table to keep %q:\n%s", want, out)
		}
	}
	for _, token := range []string{"┌", "┐", "└", "┘", "├", "┼", "┤"} {
		if !strings.Contains(out, token) {
			t.Fatalf("expected table border token %q in output:\n%s", token, out)
		}
	}
}

func TestRenderMarkdownTableBalancesDynamicColumnWidths(t *testing.T) {
	out := RenderMarkdownWithWidth(strings.Join([]string{
		"| 工具 | 文件 | 能力 |",
		"| --- | --- | --- |",
		"| **read** | `read.ts` | 读取文件内容 |",
		"| **write** | `write.ts` | 写入/覆盖文件 |",
		"| **edit / edit-diff** | `edit.ts / edit-diff.ts` | 精确文本替换 / diff 模式修改 |",
		"| **bash** | `bash.ts` | 执行 shell 命令 |",
	}, "\n"), 100)
	plain := StripANSI(out)
	widths := renderedTableColumnWidths(plain)

	if len(widths) != 3 {
		t.Fatalf("expected three table columns, got %v in:\n%s", widths, out)
	}
	if widths[0] > 24 || widths[1] > 32 {
		t.Fatalf("short token columns should stay compact, got widths %v in:\n%s", widths, out)
	}
	if widths[2] < 24 {
		t.Fatalf("description column should receive enough width, got widths %v in:\n%s", widths, out)
	}
	for _, line := range strings.Split(plain, "\n") {
		if line == "" {
			continue
		}
		if got := CellWidth(line); got != 100 {
			t.Fatalf("table line should fill target width, got %d want 100\n%s", got, out)
		}
	}
}

func TestRenderMarkdownNormalizesCompactOneLineTable(t *testing.T) {
	out := RenderMarkdownWithWidth("| 序号 | 项目 | 状态 | |:---:|:---|:---:| | 1 | 用户中心重构 | 进行中 | | 2 | 支付模块升级 | 待评审 |", 80)

	if strings.Contains(out, "|:---:|") || strings.Contains(out, "| 1 |") {
		t.Fatalf("expected compact markdown table to be normalized before rendering:\n%s", out)
	}
	for _, token := range []string{"┌", "┐", "└", "┘", "├", "┼", "┤"} {
		if !strings.Contains(out, token) {
			t.Fatalf("expected normalized compact table to render border token %q:\n%s", token, out)
		}
	}
}

func renderedTableColumnWidths(rendered string) []int {
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "┌") && strings.Contains(line, "┬") {
			parts := strings.FieldsFunc(line, func(r rune) bool {
				return r == '┌' || r == '┬' || r == '┐'
			})
			widths := make([]int, 0, len(parts))
			for _, part := range parts {
				if part != "" {
					widths = append(widths, CellWidth(part))
				}
			}
			return widths
		}
	}
	return nil
}

func TestRenderMarkdownMixedContentKeepsClosedTableBorder(t *testing.T) {
	out := RenderMarkdown("## 测试\n\n| 项目 | 状态 |\n| --- | --- |\n| 表格 | 完成 |")

	if !strings.Contains(out, "测试") {
		t.Fatalf("expected heading content in output:\n%s", out)
	}
	for _, token := range []string{"┌", "┐", "└", "┘"} {
		if !strings.Contains(out, token) {
			t.Fatalf("expected table border token %q in output:\n%s", token, out)
		}
	}
}

func TestRenderMarkdownCodeBlockUsesBackground(t *testing.T) {
	out := RenderMarkdown("```go\npackage main\n```")

	if !strings.Contains(out, "package") || !strings.Contains(out, "main") {
		t.Fatalf("expected code content in output:\n%s", out)
	}
	if !strings.Contains(out, "48;2;31;35;41") {
		t.Fatalf("expected code block background color in output:\n%q", out)
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected padded code block area, got:\n%q", out)
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, codeBlockBackgroundANSI) {
			t.Fatalf("expected every code block row to start with background color, got line:\n%q\nfull output:\n%q", line, out)
		}
	}
}

func TestRenderMarkdownCodeBlockFillsTargetWidth(t *testing.T) {
	out := RenderMarkdownWithWidth("```text\nshort line\n```", 72)

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if got := CellWidth(StripANSI(line)); got != 72 {
			t.Fatalf("code block line width = %d, want 72\n%s", got, out)
		}
	}
}

func TestRenderMarkdownCodeBlockExpandsTabsAndControlChars(t *testing.T) {
	out := RenderMarkdown("```go\nfunc main() {\n\tfmt.Println(\"hi\")\n\n}\x00\n```")

	if strings.Contains(out, "\t") || strings.Contains(out, "\x00") {
		t.Fatalf("expected tabs and control chars to be normalized:\n%q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, codeBlockBackgroundANSI) {
			t.Fatalf("expected every code block row to keep background, got line:\n%q\nfull output:\n%q", line, out)
		}
	}
}

func TestNormalizeCodeBlockTextExpandsTabsByColumns(t *testing.T) {
	got := normalizeCodeBlockText("a\tb\n\tc")
	want := "a   b\n    c"

	if got != want {
		t.Fatalf("normalizeCodeBlockText() = %q, want %q", got, want)
	}
}

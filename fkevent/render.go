package fkevent

import (
	"os"
	"strings"
	"sync"

	glamour "charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"golang.org/x/term"
)

var (
	mdRenderer     *glamour.TermRenderer
	mdRendererOnce sync.Once
)

func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 100
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }
func up(u uint) *uint     { return &u }

// customDarkStyle 基于 DarkStyleConfig 全面定制配色
func customDarkStyle() glamour.TermRendererOption {
	s := styles.DarkStyleConfig

	// ── 文档 ──
	s.Document.Color = sp("#e0e0e0")

	// ── 标题 ──
	s.Heading.Color = sp("#5c9cf5")
	s.Heading.Bold = bp(true)
	// H1: 去掉背景色块，统一蓝色粗体
	s.H1 = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
		Prefix: "# ", Color: sp("#5c9cf5"), Bold: bp(true),
	}}
	s.H2.Color = sp("#5c9cf5")
	s.H3.Color = sp("#5c9cf5")
	s.H4.Color = sp("#5c9cf5")
	s.H5.Color = sp("#5c9cf5")
	s.H6 = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
		Prefix: "###### ", Color: sp("#5c9cf5"), Bold: bp(false),
	}}

	// ── 行内样式 ──
	s.Strong = ansi.StylePrimitive{Bold: bp(true), Color: sp("#9d7cd8")}
	s.Emph = ansi.StylePrimitive{Italic: bp(true), Color: sp("#e5c07b")}
	s.Strikethrough = ansi.StylePrimitive{CrossedOut: bp(true), Color: sp("#6a6a6a")}

	// ── 行内代码 ──
	s.Code = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
		Color: sp("#7fd88f"),
	}}

	// ── 引用块 ──
	s.BlockQuote = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Italic: bp(true), Color: sp("#e5c07b")},
		Indent:         up(1),
		IndentToken:    sp("┃ "),
	}

	// ── 列表 ──
	// Prefix（而非 BlockPrefix）才会应用 Item 自身的颜色
	s.Item = ansi.StylePrimitive{Prefix: "• ", Color: sp("#fab283")}
	s.Enumeration = ansi.StylePrimitive{BlockPrefix: ". ", Color: sp("#56b6c2")}
	s.Task = ansi.StyleTask{Ticked: "[✓] ", Unticked: "[ ] "}

	// ── 链接 ──
	s.Link = ansi.StylePrimitive{Color: sp("#fab283"), Underline: bp(true)}
	s.LinkText = ansi.StylePrimitive{Color: sp("#56b6c2"), Bold: bp(true)}

	// ── 图片 ──
	s.Image = ansi.StylePrimitive{Color: sp("#fab283"), Underline: bp(true)}
	s.ImageText = ansi.StylePrimitive{Color: sp("#56b6c2"), Format: "Image: {{.text}} →"}

	// ── 分割线 ──
	s.HorizontalRule = ansi.StylePrimitive{
		Color:  sp("#6a6a6a"),
		Format: "\n──────────────────────────────────────────\n",
	}

	// ── 表格：Unicode 分隔符 ──
	s.Table = ansi.StyleTable{
		CenterSeparator: sp("┼"),
		ColumnSeparator: sp("│"),
		RowSeparator:    sp("─"),
	}

	// ── 定义列表 ──
	s.DefinitionDescription = ansi.StylePrimitive{BlockPrefix: "\n❯ "}

	// ── 代码块 + 语法高亮 ──
	s.CodeBlock = ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: sp("#e0e0e0")},
			Margin:         up(1),
		},
		Chroma: &ansi.Chroma{
			Text:                ansi.StylePrimitive{Color: sp("#e0e0e0")},
			Error:               ansi.StylePrimitive{Color: sp("#e06c75"), BackgroundColor: sp("#3c2020")},
			Comment:             ansi.StylePrimitive{Color: sp("#6a6a6a")},
			CommentPreproc:      ansi.StylePrimitive{Color: sp("#fab283")},
			Keyword:             ansi.StylePrimitive{Color: sp("#5c9cf5")},
			KeywordReserved:     ansi.StylePrimitive{Color: sp("#9d7cd8")},
			KeywordNamespace:    ansi.StylePrimitive{Color: sp("#e06c75")},
			KeywordType:         ansi.StylePrimitive{Color: sp("#e5c07b")},
			Operator:            ansi.StylePrimitive{Color: sp("#56b6c2")},
			Punctuation:         ansi.StylePrimitive{Color: sp("#abb2bf")},
			Name:                ansi.StylePrimitive{Color: sp("#e0e0e0")},
			NameBuiltin:         ansi.StylePrimitive{Color: sp("#56b6c2")},
			NameTag:             ansi.StylePrimitive{Color: sp("#e06c75")},
			NameAttribute:       ansi.StylePrimitive{Color: sp("#e5c07b")},
			NameClass:           ansi.StylePrimitive{Color: sp("#e5c07b"), Bold: bp(true), Underline: bp(true)},
			NameConstant:        ansi.StylePrimitive{Color: sp("#fab283")},
			NameDecorator:       ansi.StylePrimitive{Color: sp("#e5c07b")},
			NameFunction:        ansi.StylePrimitive{Color: sp("#fab283")},
			LiteralNumber:       ansi.StylePrimitive{Color: sp("#9d7cd8")},
			LiteralString:       ansi.StylePrimitive{Color: sp("#7fd88f")},
			LiteralStringEscape: ansi.StylePrimitive{Color: sp("#56b6c2")},
			GenericDeleted:      ansi.StylePrimitive{Color: sp("#e06c75")},
			GenericEmph:         ansi.StylePrimitive{Italic: bp(true)},
			GenericInserted:     ansi.StylePrimitive{Color: sp("#7fd88f")},
			GenericStrong:       ansi.StylePrimitive{Bold: bp(true)},
			GenericSubheading:   ansi.StylePrimitive{Color: sp("#6a6a6a")},
			Background:          ansi.StylePrimitive{BackgroundColor: sp("#2d2d2d")},
		},
	}

	return glamour.WithStyles(s)
}

func initRenderer() *glamour.TermRenderer {
	mdRendererOnce.Do(func() {
		w := termWidth() - 4
		if w < 40 {
			w = 40
		}
		r, err := glamour.NewTermRenderer(
			customDarkStyle(),
			glamour.WithWordWrap(w),
			glamour.WithEmoji(),
			glamour.WithChromaFormatter("terminal16m"),
		)
		if err != nil {
			r, _ = glamour.NewTermRenderer(
				customDarkStyle(),
				glamour.WithWordWrap(w),
				glamour.WithChromaFormatter("terminal16m"),
			)
		}
		mdRenderer = r
	})
	return mdRenderer
}

// RenderMarkdown 渲染 Markdown 为 ANSI 输出，失败时返回原文
func RenderMarkdown(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	content = strings.ReplaceAll(content, "[^", `\[^`)
	r := initRenderer()
	if r == nil {
		return content
	}
	out, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.Trim(out, "\n")
}

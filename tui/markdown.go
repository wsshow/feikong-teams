package tui

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"unicode"

	glamour "charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/alecthomas/chroma/v2/quick"
	classiclipgloss "github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var (
	mdRenderers  = map[int]*glamour.TermRenderer{}
	mdRendererMu sync.Mutex
)

func TermWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 100
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }
func up(u uint) *uint     { return &u }

const codeBlockBackground = "#1f2329"
const codeBlockBackgroundANSI = "\x1b[48;2;31;35;41m"

func codeToken(fg string) ansi.StylePrimitive {
	return ansi.StylePrimitive{Color: sp(fg), BackgroundColor: sp(codeBlockBackground)}
}

func codeTokenStyle(fg string, opts ...func(*ansi.StylePrimitive)) ansi.StylePrimitive {
	p := codeToken(fg)
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

func tokenBold(p *ansi.StylePrimitive) {
	p.Bold = bp(true)
}

func tokenItalic(p *ansi.StylePrimitive) {
	p.Italic = bp(true)
}

func tokenUnderline(p *ansi.StylePrimitive) {
	p.Underline = bp(true)
}

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
			StylePrimitive: ansi.StylePrimitive{
				Color:           sp("#e0e0e0"),
				BackgroundColor: sp(codeBlockBackground),
			},
			Indent:      up(1),
			IndentToken: sp("▌ "),
			Margin:      up(1),
		},
		Chroma: &ansi.Chroma{
			Text:                codeToken("#e0e0e0"),
			Error:               ansi.StylePrimitive{Color: sp("#e06c75"), BackgroundColor: sp("#3c2020")},
			Comment:             codeToken("#6a6a6a"),
			CommentPreproc:      codeToken("#fab283"),
			Keyword:             codeToken("#5c9cf5"),
			KeywordReserved:     codeToken("#9d7cd8"),
			KeywordNamespace:    codeToken("#e06c75"),
			KeywordType:         codeToken("#e5c07b"),
			Operator:            codeToken("#56b6c2"),
			Punctuation:         codeToken("#abb2bf"),
			Name:                codeToken("#e0e0e0"),
			NameBuiltin:         codeToken("#56b6c2"),
			NameTag:             codeToken("#e06c75"),
			NameAttribute:       codeToken("#e5c07b"),
			NameClass:           codeTokenStyle("#e5c07b", tokenBold, tokenUnderline),
			NameConstant:        codeToken("#fab283"),
			NameDecorator:       codeToken("#e5c07b"),
			NameFunction:        codeToken("#fab283"),
			LiteralNumber:       codeToken("#9d7cd8"),
			LiteralString:       codeToken("#7fd88f"),
			LiteralStringEscape: codeToken("#56b6c2"),
			GenericDeleted:      codeToken("#e06c75"),
			GenericEmph:         codeTokenStyle("#e0e0e0", tokenItalic),
			GenericInserted:     codeToken("#7fd88f"),
			GenericStrong:       codeTokenStyle("#e0e0e0", tokenBold),
			GenericSubheading:   codeToken("#6a6a6a"),
			Background:          ansi.StylePrimitive{BackgroundColor: sp(codeBlockBackground)},
		},
	}

	return glamour.WithStyles(s)
}

func rendererForWidth(width int) *glamour.TermRenderer {
	width = normalizeMarkdownWidth(width)
	mdRendererMu.Lock()
	defer mdRendererMu.Unlock()
	if r := mdRenderers[width]; r != nil {
		return r
	}
	r, err := glamour.NewTermRenderer(
		customDarkStyle(),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
		glamour.WithChromaFormatter("terminal16m"),
	)
	if err != nil {
		r, _ = glamour.NewTermRenderer(
			customDarkStyle(),
			glamour.WithWordWrap(width),
			glamour.WithChromaFormatter("terminal16m"),
		)
	}
	mdRenderers[width] = r
	return r
}

func normalizeMarkdownWidth(width int) int {
	if width <= 0 {
		width = TermWidth() - 4
	}
	if width < 20 {
		return 20
	}
	return width
}

// RenderMarkdown 渲染 Markdown 为 ANSI 输出，失败时返回原文
func RenderMarkdown(content string) string {
	return RenderMarkdownWithWidth(content, TermWidth()-4)
}

// RenderMarkdownWithWidth 按指定终端宽度渲染 Markdown，失败时返回原文
func RenderMarkdownWithWidth(content string, width int) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	width = normalizeMarkdownWidth(width)
	content = strings.ReplaceAll(content, "[^", `\[^`)
	content = normalizeCompactMarkdownTables(content)
	if segments := splitMarkdownSegments(content); hasSpecialMarkdownSegment(segments) {
		return renderMarkdownSegments(segments, width)
	}
	r := rendererForWidth(width)
	if r == nil {
		return content
	}
	out, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.Trim(out, "\n")
}

type markdownSegment struct {
	text  string
	table bool
	code  bool
}

func hasSpecialMarkdownSegment(segments []markdownSegment) bool {
	for _, seg := range segments {
		if seg.table || seg.code {
			return true
		}
	}
	return false
}

func renderMarkdownSegments(segments []markdownSegment, width int) string {
	var rendered []string
	for _, seg := range segments {
		text := strings.TrimSpace(seg.text)
		if text == "" {
			continue
		}
		if seg.table {
			rendered = append(rendered, renderMarkdownTable(text, width))
			continue
		}
		if seg.code {
			rendered = append(rendered, renderCodeBlock(text, width))
			continue
		}
		r := rendererForWidth(width)
		if r == nil {
			rendered = append(rendered, text)
			continue
		}
		out, err := r.Render(text)
		if err != nil {
			rendered = append(rendered, text)
			continue
		}
		rendered = append(rendered, strings.Trim(out, "\n"))
	}
	return strings.Join(rendered, "\n\n")
}

func splitMarkdownSegments(content string) []markdownSegment {
	lines := strings.Split(content, "\n")
	var segments []markdownSegment
	var normal []string

	flushNormal := func() {
		text := strings.TrimSpace(strings.Join(normal, "\n"))
		if text != "" {
			segments = append(segments, markdownSegment{text: text})
		}
		normal = nil
	}

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			flushNormal()
			fence := "```"
			if strings.HasPrefix(trimmed, "~~~") {
				fence = "~~~"
			}
			start := i
			i++
			for i < len(lines) {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
					i++
					break
				}
				i++
			}
			segments = append(segments, markdownSegment{
				text: strings.Join(lines[start:i], "\n"),
				code: true,
			})
			continue
		}
		if i+1 < len(lines) && isMarkdownTableRow(lines[i]) && isMarkdownTableSeparator(lines[i+1]) {
			flushNormal()
			start := i
			i += 2
			for i < len(lines) && isMarkdownTableRow(lines[i]) && strings.TrimSpace(lines[i]) != "" {
				i++
			}
			segments = append(segments, markdownSegment{
				text:  strings.Join(lines[start:i], "\n"),
				table: true,
			})
			continue
		}
		normal = append(normal, lines[i])
		i++
	}
	flushNormal()
	return segments
}

func isMarkdownTableRow(line string) bool {
	return strings.Contains(line, "|")
}

func isMarkdownTableSeparator(line string) bool {
	cells := splitMarkdownTableLine(line)
	if len(cells) < 2 {
		return false
	}
	for _, cell := range cells {
		if !isMarkdownTableSeparatorCell(cell) {
			return false
		}
	}
	return true
}

func isMarkdownTableSeparatorCell(cell string) bool {
	cell = strings.TrimSpace(cell)
	if strings.Count(cell, "-") < 3 {
		return false
	}
	for _, r := range cell {
		if r != '-' && r != ':' {
			return false
		}
	}
	return true
}

func normalizeCompactMarkdownTables(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if normalized, ok := normalizeCompactMarkdownTableLine(line); ok {
			lines[i] = normalized
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeCompactMarkdownTableLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || strings.Count(trimmed, "|") < 6 {
		return line, false
	}
	cells := splitMarkdownTableLine(trimmed)
	cells = compactMarkdownNonEmptyCells(cells)
	if len(cells) < 4 {
		return line, false
	}
	for start := 1; start < len(cells)-1; start++ {
		if !isMarkdownTableSeparatorCell(cells[start]) {
			continue
		}
		end := start
		for end < len(cells) && isMarkdownTableSeparatorCell(cells[end]) {
			end++
		}
		colCount := end - start
		if colCount < 2 || start != colCount || (len(cells)-end)%colCount != 0 {
			start = end - 1
			continue
		}
		rows := [][]string{cells[:start], cells[start:end]}
		for pos := end; pos < len(cells); pos += colCount {
			rows = append(rows, cells[pos:pos+colCount])
		}
		var normalized []string
		for _, row := range rows {
			if !compactMarkdownRowHasContent(row) {
				return line, false
			}
			normalized = append(normalized, "| "+strings.Join(row, " | ")+" |")
		}
		return strings.Join(normalized, "\n"), true
	}
	return line, false
}

func compactMarkdownRowHasContent(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return true
		}
	}
	return false
}

func compactMarkdownNonEmptyCells(cells []string) []string {
	nonEmpty := make([]string, 0, len(cells))
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			nonEmpty = append(nonEmpty, cell)
		}
	}
	return nonEmpty
}

func splitMarkdownTableLine(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")

	var cells []string
	var b strings.Builder
	escaped := false
	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '|' {
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
			continue
		}
		b.WriteRune(r)
	}
	cells = append(cells, strings.TrimSpace(b.String()))
	return cells
}

func renderCodeBlock(codeMarkdown string, width int) string {
	lang, code := parseCodeBlock(codeMarkdown)
	highlighted := highlightCode(code, lang)
	return paintCodeBlockBackground(highlighted, width)
}

func parseCodeBlock(codeMarkdown string) (lang string, code string) {
	lines := strings.Split(strings.Trim(codeMarkdown, "\n"), "\n")
	if len(lines) == 0 {
		return "", ""
	}
	opener := strings.TrimSpace(lines[0])
	fence := "```"
	if strings.HasPrefix(opener, "~~~") {
		fence = "~~~"
	}
	lang = strings.TrimSpace(strings.TrimPrefix(opener, fence))
	if fields := strings.Fields(lang); len(fields) > 0 {
		lang = fields[0]
	}
	body := lines[1:]
	if len(body) > 0 && strings.HasPrefix(strings.TrimSpace(body[len(body)-1]), fence) {
		body = body[:len(body)-1]
	}
	return lang, strings.Join(body, "\n")
}

func highlightCode(code, lang string) string {
	code = normalizeCodeBlockText(code)
	if strings.TrimSpace(code) == "" {
		return ""
	}
	lexer := lang
	if lexer == "" {
		lexer = "text"
	}
	var out bytes.Buffer
	if err := quick.Highlight(&out, code, lexer, "terminal16m", "monokai"); err != nil {
		return code
	}
	return strings.TrimRight(out.String(), "\n")
}

func normalizeCodeBlockText(code string) string {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	code = strings.ReplaceAll(code, "\r", "\n")

	const tabSize = 4
	var b strings.Builder
	col := 0
	for _, r := range code {
		switch {
		case r == '\n':
			b.WriteRune(r)
			col = 0
		case r == '\t':
			spaces := tabSize - col%tabSize
			b.WriteString(strings.Repeat(" ", spaces))
			col += spaces
		case unicode.IsControl(r):
			b.WriteRune(' ')
			col++
		default:
			b.WriteRune(r)
			width := runewidth.RuneWidth(r)
			if width < 1 {
				width = 1
			}
			col += width
		}
	}
	return b.String()
}

func paintCodeBlockBackground(highlighted string, width int) string {
	if width < 20 {
		width = 20
	}
	contentWidth := width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	lines := strings.Split(highlighted, "\n")
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, paintCodeBlockLine("", contentWidth))
	for _, line := range lines {
		rows = append(rows, paintCodeBlockLine("  "+keepBackgroundAfterReset(line), contentWidth))
	}
	rows = append(rows, paintCodeBlockLine("", contentWidth))
	return strings.Join(rows, "\n")
}

func paintCodeBlockLine(line string, contentWidth int) string {
	visibleWidth := runewidthStringWidth(line)
	if visibleWidth < contentWidth {
		line += strings.Repeat(" ", contentWidth-visibleWidth)
	}
	return codeBlockBackgroundANSI + line + "\x1b[0m"
}

func keepBackgroundAfterReset(s string) string {
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+codeBlockBackgroundANSI)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+codeBlockBackgroundANSI)
	return s
}

type tableAlign int

const (
	tableAlignLeft tableAlign = iota
	tableAlignCenter
	tableAlignRight
)

func parseTableAligns(separator []string, count int) []tableAlign {
	aligns := make([]tableAlign, count)
	for i := 0; i < count && i < len(separator); i++ {
		cell := strings.TrimSpace(separator[i])
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		switch {
		case left && right:
			aligns[i] = tableAlignCenter
		case right:
			aligns[i] = tableAlignRight
		default:
			aligns[i] = tableAlignLeft
		}
	}
	return aligns
}

func renderMarkdownTable(tableMarkdown string, width int) string {
	lines := strings.Split(strings.TrimSpace(tableMarkdown), "\n")
	if len(lines) < 2 {
		return tableMarkdown
	}

	rows := make([][]string, 0, len(lines)-1)
	header := splitMarkdownTableLine(lines[0])
	colCount := len(header)
	for _, line := range lines[2:] {
		cells := splitMarkdownTableLine(line)
		if len(cells) > colCount {
			colCount = len(cells)
		}
		rows = append(rows, cells)
	}
	normalizeTableRow(&header, colCount)
	for i := range rows {
		normalizeTableRow(&rows[i], colCount)
	}
	aligns := parseTableAligns(splitMarkdownTableLine(lines[1]), colCount)

	t := table.New().
		Border(classiclipgloss.NormalBorder()).
		BorderStyle(classiclipgloss.NewStyle().Foreground(classiclipgloss.Color("8"))).
		BorderTop(true).
		BorderBottom(true).
		BorderLeft(true).
		BorderRight(true).
		BorderHeader(true).
		BorderColumn(true).
		BorderRow(true).
		Wrap(true).
		Headers(header...).
		Rows(rows...).
		StyleFunc(func(row, col int) classiclipgloss.Style {
			style := classiclipgloss.NewStyle().Padding(0, 1)
			if row == table.HeaderRow {
				return style.Bold(true).Foreground(classiclipgloss.Color("12")).Align(classiclipgloss.Center)
			}
			if col < len(aligns) {
				return style.Align(tableAlignPosition(aligns[col]))
			}
			return style
		})

	rendered := t.String()
	maxWidth := normalizeMarkdownWidth(width)
	if classiclipgloss.Width(rendered) > maxWidth {
		rendered = t.Width(maxWidth).String()
	}
	return strings.TrimRight(rendered, "\n")
}

func normalizeTableRow(row *[]string, count int) {
	for len(*row) < count {
		*row = append(*row, "")
	}
	if len(*row) > count {
		*row = (*row)[:count]
	}
}

func tableAlignPosition(align tableAlign) classiclipgloss.Position {
	switch align {
	case tableAlignRight:
		return classiclipgloss.Right
	case tableAlignCenter:
		return classiclipgloss.Center
	default:
		return classiclipgloss.Left
	}
}

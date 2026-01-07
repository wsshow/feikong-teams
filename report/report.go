package report

import (
	"os"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func ConvertMarkdownToHTML(md []byte) (bsHTML []byte) {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

func ConvertMarkdownFileToHTML(mdFilePath string) (bsHTML []byte, err error) {
	md, err := os.ReadFile(mdFilePath)
	if err != nil {
		return nil, err
	}
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer), nil
}

func ConvertMarkdownFileToHTMLFile(mdFilePath string) (htmlFilePath string, err error) {
	md, err := os.ReadFile(mdFilePath)
	if err != nil {
		return "", err
	}
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	htmlByteData := markdown.Render(doc, renderer)
	htmlFilePath = strings.TrimSuffix(mdFilePath, ".md") + ".html"
	err = os.WriteFile(htmlFilePath, htmlByteData, 0644)
	if err != nil {
		return "", err
	}
	return htmlFilePath, nil
}

package report

import (
	"html/template"
	"os"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func ConvertMarkdownToHTML(markdownByteData []byte) (htmlByteData []byte) {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(markdownByteData)

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

func ConvertMarkdownFileToHTML(mdFilePath string) (htmlByteData []byte, err error) {
	md, err := os.ReadFile(mdFilePath)
	if err != nil {
		return nil, err
	}
	return ConvertMarkdownToHTML(md), nil
}

func ConvertMarkdownFileToHTMLFile(mdFilePath string) (htmlFilePath string, err error) {
	htmlByteData, err := ConvertMarkdownFileToHTML(mdFilePath)
	if err != nil {
		return "", err
	}
	htmlFilePath = strings.TrimSuffix(mdFilePath, ".md") + ".html"
	err = os.WriteFile(htmlFilePath, htmlByteData, 0644)
	if err != nil {
		return "", err
	}
	return htmlFilePath, nil
}

func ConvertMarkdownFileToNiceHTMLFile(mdFilePath string) (htmlFilePath string, err error) {
	htmlByteData, err := ConvertMarkdownFileToHTML(mdFilePath)
	if err != nil {
		return "", err
	}

	htmlFilePath = strings.TrimSuffix(mdFilePath, ".md") + ".html"
	f, err := os.Create(htmlFilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tmpl, _ := template.New("fkteams_report").Parse(htmlTemplate)
	data := struct {
		Content template.HTML
	}{
		Content: template.HTML(string(htmlByteData)),
	}

	err = tmpl.Execute(f, data)
	if err != nil {
		return "", err
	}

	return htmlFilePath, nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>非空团队 - 历史记录</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Oxygen", "Ubuntu", "Cantarell", "Fira Sans", "Droid Sans", "Helvetica Neue", sans-serif;
            font-size: 16px;
            line-height: 1.8;
            color: #2c3e50;
            background: #f8f9fa;
            padding: 20px;
        }

        .markdown-body {
            max-width: 900px;
            margin: 0 auto;
            background: #ffffff;
            padding: 50px 60px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
        }

        /* 标题样式 */
        .markdown-body h1,
        .markdown-body h2,
        .markdown-body h3,
        .markdown-body h4,
        .markdown-body h5,
        .markdown-body h6 {
            font-weight: 600;
            line-height: 1.4;
            margin-top: 24px;
            margin-bottom: 16px;
            color: #1a202c;
        }

        .markdown-body h1 {
            font-size: 2em;
            padding-bottom: 12px;
            border-bottom: 2px solid #e2e8f0;
            margin-top: 0;
        }

        .markdown-body h2 {
            font-size: 1.6em;
            padding-bottom: 10px;
            border-bottom: 1px solid #edf2f7;
        }

        .markdown-body h3 {
            font-size: 1.4em;
        }

        .markdown-body h4 {
            font-size: 1.2em;
        }

        .markdown-body h5 {
            font-size: 1.1em;
        }

        .markdown-body h6 {
            font-size: 1em;
            color: #4a5568;
        }

        /* 段落样式 */
        .markdown-body p {
            margin-top: 0;
            margin-bottom: 16px;
        }

        /* 链接样式 */
        .markdown-body a {
            color: #3182ce;
            text-decoration: none;
            border-bottom: 1px solid transparent;
            transition: all 0.2s ease;
        }

        .markdown-body a:hover {
            color: #2c5aa0;
            border-bottom-color: #3182ce;
        }

        /* 列表样式 */
        .markdown-body ul,
        .markdown-body ol {
            margin-top: 0;
            margin-bottom: 16px;
            padding-left: 2em;
        }

        .markdown-body ul ul,
        .markdown-body ol ol,
        .markdown-body ul ol,
        .markdown-body ol ul {
            margin-top: 4px;
            margin-bottom: 4px;
        }

        .markdown-body li {
            margin-bottom: 4px;
        }

        .markdown-body li > p {
            margin-bottom: 8px;
        }

        /* 行内代码样式 */
        .markdown-body code {
            background: #f7fafc;
            color: #d73a49;
            padding: 2px 6px;
            border-radius: 4px;
            font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, Courier, monospace;
            font-size: 0.9em;
            border: 1px solid #e2e8f0;
        }

        /* 代码块样式 */
        .markdown-body pre {
            background: #2d3748;
            color: #e2e8f0;
            padding: 20px;
            border-radius: 6px;
            overflow-x: auto;
            margin-top: 0;
            margin-bottom: 16px;
            line-height: 1.6;
            font-size: 0.9em;
        }

        .markdown-body pre code {
            background: transparent;
            color: inherit;
            padding: 0;
            border-radius: 0;
            font-size: 1em;
            border: none;
        }

        /* 引用块样式 */
        .markdown-body blockquote {
            border-left: 4px solid #cbd5e0;
            background: #f7fafc;
            margin: 16px 0;
            padding: 12px 20px;
            color: #4a5568;
            border-radius: 0 4px 4px 0;
        }

        .markdown-body blockquote > :first-child {
            margin-top: 0;
        }

        .markdown-body blockquote > :last-child {
            margin-bottom: 0;
        }

        /* 表格样式 */
        .markdown-body table {
            border-collapse: collapse;
            width: 100%;
            margin-top: 0;
            margin-bottom: 16px;
            overflow: hidden;
            border-radius: 6px;
            border: 1px solid #e2e8f0;
        }

        .markdown-body table th,
        .markdown-body table td {
            padding: 12px 16px;
            border: 1px solid #e2e8f0;
            text-align: left;
        }

        .markdown-body table th {
            background: #f7fafc;
            font-weight: 600;
            color: #2d3748;
        }

        .markdown-body table tr:nth-child(even) {
            background: #f7fafc;
        }

        .markdown-body table tr:hover {
            background: #edf2f7;
        }

        /* 水平分割线 */
        .markdown-body hr {
            height: 0;
            border: 0;
            border-top: 2px solid #e2e8f0;
            margin: 24px 0;
        }

        /* 图片样式 */
        .markdown-body img {
            max-width: 100%;
            height: auto;
            border-radius: 6px;
            margin: 16px 0;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
        }

        /* 强调样式 */
        .markdown-body strong {
            font-weight: 600;
            color: #1a202c;
        }

        .markdown-body em {
            font-style: italic;
            color: #4a5568;
        }

        /* 删除线 */
        .markdown-body del {
            text-decoration: line-through;
            color: #718096;
        }

        /* 任务列表 */
        .markdown-body input[type="checkbox"] {
            margin-right: 8px;
        }

        /* 响应式设计 */
        @media (max-width: 768px) {
            body {
                padding: 10px;
            }

            .markdown-body {
                padding: 30px 20px;
            }

            .markdown-body h1 {
                font-size: 1.6em;
            }

            .markdown-body h2 {
                font-size: 1.4em;
            }

            .markdown-body h3 {
                font-size: 1.2em;
            }

            .markdown-body table {
                font-size: 0.9em;
            }

            .markdown-body table th,
            .markdown-body table td {
                padding: 8px 12px;
            }
        }

        /* 打印样式优化 */
        @media print {
            body {
                background: #ffffff;
            }

            .markdown-body {
                box-shadow: none;
                padding: 0;
            }
        }
    </style>
</head>
<body>
    <div class="markdown-body">
        {{.Content}}
    </div>
</body>
</html>
`

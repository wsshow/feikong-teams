package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
)

const (
	// MaxResponseSize 最大响应体大小 (5MB)
	MaxResponseSize = 5 * 1024 * 1024
	// MaxTimeout 最大超时时间 (120秒)
	MaxTimeout = 120
	// DefaultTimeout 默认超时时间 (30秒)
	DefaultTimeout = 30
)

// FetchRequest HTTP请求参数
type FetchRequest struct {
	URL     string `json:"url" jsonschema:"required,description:要获取内容的URL地址(必须以http://或https://开头)"`
	Format  string `json:"format,omitempty" jsonschema:"description:返回内容的格式(text/markdown/html/json),默认text。text格式会自动从HTML中提取纯文本,markdown格式会将HTML转换为markdown,html格式返回原始HTML,json格式保持JSON原样"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description:请求超时时间(秒),默认30秒,最大120秒"`
}

// FetchResponse HTTP响应
type FetchResponse struct {
	Content      string `json:"content" jsonschema:"description:响应内容(根据format参数处理后的内容)"`
	StatusCode   int    `json:"status_code,omitempty" jsonschema:"description:HTTP状态码"`
	ContentType  string `json:"content_type,omitempty" jsonschema:"description:原始内容类型"`
	IsTruncated  bool   `json:"is_truncated,omitempty" jsonschema:"description:内容是否被截断"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description:错误信息"`
}

// Fetch 发送HTTP请求获取网络资源
func Fetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error) {
	// 参数验证
	if req.URL == "" {
		return &FetchResponse{ErrorMessage: "URL is required"}, nil
	}

	// 验证URL协议
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return &FetchResponse{ErrorMessage: "URL must start with http:// or https://"}, nil
	}

	// 设置默认值和限制
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	} else if req.Timeout > MaxTimeout {
		req.Timeout = MaxTimeout
	}

	// 设置默认格式
	format := strings.ToLower(req.Format)
	if format == "" {
		format = "text"
	}
	if format != "text" && format != "markdown" && format != "html" && format != "json" {
		return &FetchResponse{ErrorMessage: "format must be one of: text, markdown, html, json"}, nil
	}

	// 创建HTTP客户端
	client := createHTTPClient(req.Timeout)

	// 创建请求
	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	if err != nil {
		return &FetchResponse{ErrorMessage: fmt.Sprintf("failed to create request: %v", err)}, nil
	}

	// 设置User-Agent
	httpReq.Header.Set("User-Agent", "FKTEAMS/1.0")

	// 发送请求
	resp, err := client.Do(httpReq)
	if err != nil {
		return &FetchResponse{ErrorMessage: fmt.Sprintf("failed to fetch URL: %v", err)}, nil
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return &FetchResponse{
			StatusCode:   resp.StatusCode,
			ErrorMessage: fmt.Sprintf("request failed with status code: %d", resp.StatusCode),
		}, nil
	}

	// 读取响应体(限制大小)
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseSize))
	if err != nil {
		return &FetchResponse{
			StatusCode:   resp.StatusCode,
			ErrorMessage: fmt.Sprintf("failed to read response body: %v", err),
		}, nil
	}

	content := string(body)
	contentType := resp.Header.Get("Content-Type")

	// 验证UTF-8编码
	if !utf8.ValidString(content) {
		return &FetchResponse{
			StatusCode:   resp.StatusCode,
			ContentType:  contentType,
			ErrorMessage: "response content is not valid UTF-8",
		}, nil
	}

	// 根据格式处理内容
	processedContent, err := processContent(content, contentType, format)
	if err != nil {
		return &FetchResponse{
			StatusCode:   resp.StatusCode,
			ContentType:  contentType,
			ErrorMessage: fmt.Sprintf("failed to process content: %v", err),
		}, nil
	}

	// 检查是否截断
	isTruncated := int64(len(body)) >= MaxResponseSize

	return &FetchResponse{
		Content:     processedContent,
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		IsTruncated: isTruncated,
	}, nil
}

// processContent 根据格式处理内容
func processContent(content, contentType, format string) (string, error) {
	isHTML := strings.Contains(contentType, "text/html")

	switch format {
	case "text":
		if isHTML {
			return extractTextFromHTML(content)
		}
		return content, nil

	case "markdown":
		if isHTML {
			return convertHTMLToMarkdown(content)
		}
		// 非HTML内容用代码块包裹
		return "```\n" + content + "\n```", nil

	case "html":
		if isHTML {
			// 只返回body部分
			return extractHTMLBody(content)
		}
		return content, nil

	case "json":
		return content, nil

	default:
		return content, nil
	}
}

// extractTextFromHTML 从HTML中提取纯文本
func extractTextFromHTML(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// 移除script和style标签
	doc.Find("script, style").Remove()

	// 提取文本内容
	text := doc.Find("body").Text()

	// 清理多余空白
	text = strings.TrimSpace(text)
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n"), nil
}

// convertHTMLToMarkdown 将HTML转换为Markdown
func convertHTMLToMarkdown(html string) (string, error) {
	markdown, err := md.ConvertString(html)
	if err != nil {
		return "", fmt.Errorf("failed to convert HTML to markdown: %w", err)
	}

	return markdown, nil
}

// extractHTMLBody 提取HTML的body部分
func extractHTMLBody(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	body, err := doc.Find("body").Html()
	if err != nil {
		return "", fmt.Errorf("failed to extract body: %w", err)
	}

	if body == "" {
		return html, nil // 如果没有body标签,返回原始内容
	}

	return "<html>\n<body>\n" + body + "\n</body>\n</html>", nil
}

// createHTTPClient 创建HTTP客户端
func createHTTPClient(timeoutSec int) *http.Client {
	proxyStr := os.Getenv("FEIKONG_PROXY_URL")
	var proxyFunc func(*http.Request) (*url.URL, error)

	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err == nil {
			proxyFunc = http.ProxyURL(proxyURL)
		}
	} else {
		proxyFunc = http.ProxyFromEnvironment
	}

	transport := &http.Transport{
		Proxy:                 proxyFunc,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSec) * time.Second,
	}
}

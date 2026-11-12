package doc

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"fkteams/internal/runtime/pathguard"

	"github.com/wsshow/docreader"
)

// GetDocumentInfoRequest 获取文档信息请求
type GetDocumentInfoRequest struct {
	FilePath string `json:"file_path" jsonschema:"required,description:文档文件路径（支持 .docx、.pdf、.xlsx、.pptx、.txt、.csv、.md、.rtf）"`
}

// GetDocumentInfoResponse 获取文档信息响应
type GetDocumentInfoResponse struct {
	FilePath      string            `json:"file_path" jsonschema:"description:文件路径"`
	FileType      string            `json:"file_type" jsonschema:"description:文件类型（如：PDF、DOCX、XLSX等）"`
	FileSize      string            `json:"file_size" jsonschema:"description:文件大小（格式化字符串）"`
	TotalPages    int               `json:"total_pages,omitempty" jsonschema:"description:总页数（对于PDF、PPTX等分页文档）"`
	TotalSheets   int               `json:"total_sheets,omitempty" jsonschema:"description:工作表总数（仅XLSX）"`
	SheetNames    []string          `json:"sheet_names,omitempty" jsonschema:"description:工作表名称列表（仅XLSX）"`
	EstimatedSize string            `json:"estimated_size" jsonschema:"description:预估文本大小（用于评估是否适合一次性读取）"`
	Metadata      map[string]string `json:"metadata" jsonschema:"description:文档元数据（标题、作者等）"`
	ErrorMessage  string            `json:"error_message,omitempty" jsonschema:"description:错误信息"`
}

// ReadDocumentByPagesRequest 按页读取文档请求
type ReadDocumentByPagesRequest struct {
	FilePath  string `json:"file_path" jsonschema:"required,description:文档文件路径"`
	StartPage int    `json:"start_page,omitempty" jsonschema:"description:起始页码（从0开始，默认0）"`
	EndPage   *int   `json:"end_page,omitempty" jsonschema:"description:结束页码（包含，-1表示到末尾，默认-1）"`
}

// ReadDocumentByPagesResponse 按页读取文档响应
type ReadDocumentByPagesResponse struct {
	Content      string              `json:"content" jsonschema:"description:读取的文档内容"`
	Pages        []PageContentDetail `json:"pages" jsonschema:"description:分页内容详情"`
	TotalPages   int                 `json:"total_pages" jsonschema:"description:文档总页数"`
	ReadPages    int                 `json:"read_pages" jsonschema:"description:实际读取的页数"`
	Metadata     map[string]string   `json:"metadata" jsonschema:"description:文档元数据"`
	ErrorMessage string              `json:"error_message,omitempty" jsonschema:"description:错误信息"`
}

// PageContentDetail 页面内容详情
type PageContentDetail struct {
	PageNumber int    `json:"page_number" jsonschema:"description:页码（从0开始）"`
	PageName   string `json:"page_name,omitempty" jsonschema:"description:页面名称（如工作表名）"`
	LineCount  int    `json:"line_count" jsonschema:"description:该页的行数"`
}

// ReadDocumentByLinesRequest 按行读取文档请求
type ReadDocumentByLinesRequest struct {
	FilePath  string `json:"file_path" jsonschema:"required,description:文档文件路径"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"description:起始行号（从0开始，默认0）"`
	EndLine   *int   `json:"end_line,omitempty" jsonschema:"description:结束行号（包含，-1表示到末尾，默认-1）"`
	PageIndex int    `json:"page_index,omitempty" jsonschema:"description:指定页面索引（从0开始，仅对多页文档有效，-1表示第一页，默认-1）"`
}

// ReadDocumentByLinesResponse 按行读取文档响应
type ReadDocumentByLinesResponse struct {
	Content      string            `json:"content" jsonschema:"description:读取的文档内容"`
	TotalLines   int               `json:"total_lines" jsonschema:"description:该页总行数"`
	ReadLines    int               `json:"read_lines" jsonschema:"description:实际读取的行数"`
	PageIndex    int               `json:"page_index" jsonschema:"description:读取的页面索引"`
	Metadata     map[string]string `json:"metadata" jsonschema:"description:文档元数据"`
	ErrorMessage string            `json:"error_message,omitempty" jsonschema:"description:错误信息"`
}

// ReadDocumentSmartRequest 智能读取文档请求
type ReadDocumentSmartRequest struct {
	FilePath     string `json:"file_path" jsonschema:"required,description:文档文件路径"`
	MaxChars     int    `json:"max_chars,omitempty" jsonschema:"description:最大字符数限制（默认50000，建议10000-100000之间）"`
	SampleMode   bool   `json:"sample_mode,omitempty" jsonschema:"description:采样模式（true=均匀采样全文，false=从头读取，默认false）"`
	CleanContent *bool  `json:"clean_content,omitempty" jsonschema:"description:是否清理文本（移除多余空格、空行等，默认true）"`
}

// ReadDocumentSmartResponse 智能读取文档响应
type ReadDocumentSmartResponse struct {
	Content      string            `json:"content" jsonschema:"description:读取的文档内容"`
	IsTruncated  bool              `json:"is_truncated" jsonschema:"description:内容是否被截断"`
	OriginalSize int               `json:"original_size" jsonschema:"description:原始文本大小（字符数）"`
	ReturnedSize int               `json:"returned_size" jsonschema:"description:返回的文本大小（字符数）"`
	Strategy     string            `json:"strategy" jsonschema:"description:使用的读取策略"`
	Metadata     map[string]string `json:"metadata" jsonschema:"description:文档元数据"`
	ErrorMessage string            `json:"error_message,omitempty" jsonschema:"description:错误信息"`
	Suggestion   string            `json:"suggestion,omitempty" jsonschema:"description:建议（如何更好地读取该文档）"`
}

// GetDocumentInfo 获取文档基本信息
func (t *Tools) GetDocumentInfo(ctx context.Context, req *GetDocumentInfoRequest) (*GetDocumentInfoResponse, error) {
	path, info, cleanup, err := t.snapshot(ctx, req.FilePath)
	if err != nil {
		return &GetDocumentInfoResponse{ErrorMessage: fmt.Sprintf("文件访问失败，读取文档失败: %v", err)}, nil
	}
	defer cleanup()
	fileInfo := info

	// 获取文件类型
	ext := strings.ToLower(filepath.Ext(req.FilePath))
	fileType := strings.TrimPrefix(ext, ".")
	fileType = strings.ToUpper(fileType)

	// 格式化文件大小
	fileSize := formatFileSize(fileInfo.Size())

	reader := documentReader(filepath.Ext(path))
	if reader == nil {
		return &GetDocumentInfoResponse{ErrorMessage: "unsupported document format"}, nil
	}
	metadata, err := reader.GetMetadata(path)
	if err != nil {
		return &GetDocumentInfoResponse{ErrorMessage: fmt.Sprintf("读取文档失败: %v", err)}, nil
	}

	response := &GetDocumentInfoResponse{
		FilePath:      req.FilePath,
		FileType:      fileType,
		FileSize:      fileSize,
		EstimatedSize: fmt.Sprintf("文件 %s；文本大小在读取后确定", fileSize),
		Metadata:      metadata,
	}

	// 根据文件类型获取额外信息
	switch ext {
	case ".pdf":
		if pages, ok := metadata["pages"]; ok {
			fmt.Sscanf(pages, "%d", &response.TotalPages)
		}
	case ".pptx":
		if slides, ok := metadata["slide_count"]; ok {
			fmt.Sscanf(slides, "%d", &response.TotalPages)
		}
	case ".xlsx":
		if sheets, ok := metadata["sheets"]; ok {
			response.SheetNames = strings.Split(sheets, ",")
			response.TotalSheets = len(response.SheetNames)
		}
	}

	return response, nil
}

// ReadDocumentByPages 按页范围读取文档
func (t *Tools) ReadDocumentByPages(ctx context.Context, req *ReadDocumentByPagesRequest) (*ReadDocumentByPagesResponse, error) {
	path, _, cleanup, err := t.snapshot(ctx, req.FilePath)
	if err != nil {
		return &ReadDocumentByPagesResponse{ErrorMessage: fmt.Sprintf("文件访问失败，读取文档失败: %v", err)}, nil
	}
	defer cleanup()
	endPage := 999999
	if req.EndPage != nil && *req.EndPage >= 0 {
		endPage = *req.EndPage
	}
	if req.StartPage < 0 || endPage < req.StartPage {
		return &ReadDocumentByPagesResponse{ErrorMessage: "invalid page range"}, nil
	}
	config := docreader.NewReadConfig().WithPageRange(req.StartPage, endPage)

	// 读取文档
	result, err := docreader.ReadDocumentWithConfig(path, config)
	if err != nil {
		return &ReadDocumentByPagesResponse{
			ErrorMessage: fmt.Sprintf("读取文档失败: %v", err),
		}, nil
	}

	if utf8.RuneCountInString(result.Content) > maxReturnedChars {
		return &ReadDocumentByPagesResponse{ErrorMessage: "document range exceeds 100000 characters; request a smaller range or use read_document_smart"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 构建响应
	response := &ReadDocumentByPagesResponse{
		Content:    result.Content,
		TotalPages: result.TotalPages,
		ReadPages:  len(result.Pages),
		Metadata:   result.Metadata,
	}

	// 填充页面详情
	response.Pages = make([]PageContentDetail, len(result.Pages))
	for i, page := range result.Pages {
		response.Pages[i] = PageContentDetail{
			PageNumber: page.PageNumber,
			PageName:   page.PageName,
			LineCount:  page.TotalLines,
		}
	}

	return response, nil
}

// ReadDocumentByLines 按行范围读取文档
func (t *Tools) ReadDocumentByLines(ctx context.Context, req *ReadDocumentByLinesRequest) (*ReadDocumentByLinesResponse, error) {
	path, _, cleanup, err := t.snapshot(ctx, req.FilePath)
	if err != nil {
		return &ReadDocumentByLinesResponse{ErrorMessage: fmt.Sprintf("文件访问失败，读取文档失败: %v", err)}, nil
	}
	defer cleanup()
	pageIndex := req.PageIndex
	if pageIndex < 0 {
		pageIndex = 0
	}

	// 设置默认值
	endLine := 999999
	if req.EndLine != nil && *req.EndLine >= 0 {
		endLine = *req.EndLine
	}
	if req.StartLine < 0 || endLine < req.StartLine {
		return &ReadDocumentByLinesResponse{ErrorMessage: "invalid line range"}, nil
	}

	// 创建读取配置
	config := docreader.NewReadConfig().
		AddPageLineRange(pageIndex, req.StartLine, endLine)

	// 读取文档
	result, err := docreader.ReadDocumentWithConfig(path, config)
	if err != nil {
		return &ReadDocumentByLinesResponse{
			ErrorMessage: fmt.Sprintf("读取文档失败: %v", err),
		}, nil
	}

	if utf8.RuneCountInString(result.Content) > maxReturnedChars {
		return &ReadDocumentByLinesResponse{ErrorMessage: "document range exceeds 100000 characters; request a smaller range or use read_document_smart"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 构建响应
	response := &ReadDocumentByLinesResponse{
		Content:   result.Content,
		PageIndex: pageIndex,
		Metadata:  result.Metadata,
	}

	// 获取该页的行信息
	if len(result.Pages) > 0 {
		page := result.Pages[0]
		response.TotalLines = page.TotalLines
		response.ReadLines = len(page.Lines)
	}

	return response, nil
}

// ReadDocumentSmart 智能读取文档（自动适配上下文限制）
func (t *Tools) ReadDocumentSmart(ctx context.Context, req *ReadDocumentSmartRequest) (*ReadDocumentSmartResponse, error) {
	path, _, cleanup, err := t.snapshot(ctx, req.FilePath)
	if err != nil {
		return &ReadDocumentSmartResponse{ErrorMessage: fmt.Sprintf("文件访问失败，读取文档失败: %v", err)}, nil
	}
	defer cleanup()
	// 设置默认值
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = 50000 // 默认 50k 字符
	}

	if maxChars > maxReturnedChars {
		maxChars = maxReturnedChars
	}
	cleanContent := true
	if req.CleanContent != nil {
		cleanContent = *req.CleanContent
	}

	// 首先读取完整文档
	var doc *docreader.Document

	if cleanContent {
		doc, err = docreader.ReadDocumentWithClean(path)
	} else {
		doc, err = docreader.ReadDocument(path)
	}

	if err != nil {
		return &ReadDocumentSmartResponse{
			ErrorMessage: fmt.Sprintf("读取文档失败: %v", err),
		}, nil
	}

	originalSize := utf8.RuneCountInString(doc.Content)

	response := &ReadDocumentSmartResponse{
		OriginalSize: originalSize,
		Metadata:     doc.Metadata,
	}

	// 如果文档大小在限制内，直接返回全部内容
	if originalSize <= maxChars {
		response.Content = doc.Content
		response.IsTruncated = false
		response.ReturnedSize = originalSize
		response.Strategy = "完整读取"
		return response, nil
	}

	// 文档过大，需要截断
	response.IsTruncated = true

	if req.SampleMode {
		// 采样模式：从文档不同位置均匀采样
		response.Content = sampleContent(doc.Content, maxChars)
		response.Strategy = "均匀采样"
		response.Suggestion = fmt.Sprintf("文档较大（%d字符），已采样关键部分。建议使用 ReadDocumentByPages 或 ReadDocumentByLines 按需读取特定部分", originalSize)
	} else {
		// 默认：从头开始读取
		response.Content = string([]rune(doc.Content)[:maxChars])
		response.Strategy = "从头截断"
		response.Suggestion = fmt.Sprintf("文档较大（%d字符），仅返回前 %d 字符。建议使用 ReadDocumentByPages 或 ReadDocumentByLines 按需读取", originalSize, maxChars)
	}

	response.ReturnedSize = utf8.RuneCountInString(response.Content)

	return response, nil
}

// formatFileSize 格式化文件大小
func formatFileSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// sampleContent 从内容中均匀采样
func sampleContent(content string, maxChars int) string {
	chars := []rune(content)
	if maxChars <= 0 {
		return ""
	}
	if len(chars) <= maxChars {
		return content
	}
	const separator1 = "\n\n... [中间部分] ...\n\n"
	const separator2 = "\n\n... [后续部分] ...\n\n"
	available := maxChars - utf8.RuneCountInString(separator1+separator2)
	if available < 300 {
		return string(chars[:maxChars])
	}
	part := available / 3
	middle := len(chars)/2 - part/2
	return string(chars[:part]) + separator1 + string(chars[middle:middle+part]) + separator2 + string(chars[len(chars)-part:])
}

const maxDocumentBytes int64 = 32 << 20
const maxExpandedBytes uint64 = 64 << 20
const maxReturnedChars = 100000

// Tools 持有显式工作区，文档解析器只接触有大小上限的私有快照。
type Tools struct{ baseDir string }

// NewTools 创建受工作区约束的文档工具。
func NewTools(baseDir string) (*Tools, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	return &Tools{baseDir: abs}, nil
}

func (t *Tools) snapshot(ctx context.Context, path string) (string, os.FileInfo, func(), error) {
	if path == "" {
		return "", nil, nil, fmt.Errorf("document path is required")
	}
	if err := ctx.Err(); err != nil {
		return "", nil, nil, err
	}
	resolved, err := pathguard.ResolveWorkspace(t.baseDir, path)
	if err != nil {
		return "", nil, nil, err
	}
	root, err := os.OpenRoot(resolved.BaseAbs)
	if err != nil {
		return "", nil, nil, err
	}
	file, err := root.Open(resolved.RelPath)
	root.Close()
	if err != nil {
		return "", nil, nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDocumentBytes {
		return "", nil, nil, fmt.Errorf("document exceeds file limits")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !docreader.IsFormatSupported(ext) {
		return "", nil, nil, fmt.Errorf("unsupported document format")
	}
	snapshot, err := os.CreateTemp("", "fkteams-document-*"+ext)
	if err != nil {
		return "", nil, nil, err
	}
	cleanup := func() { os.Remove(snapshot.Name()) }
	size, err := io.Copy(snapshot, io.LimitReader(&contextReader{ctx, file}, maxDocumentBytes+1))
	closeErr := snapshot.Close()
	if closeErr == nil {
		closeErr = os.Chtimes(snapshot.Name(), info.ModTime(), info.ModTime())
	}
	if err == nil {
		err = closeErr
	}
	if err == nil && size > maxDocumentBytes {
		err = fmt.Errorf("document exceeds size limit")
	}
	if err == nil && (ext == ".docx" || ext == ".xlsx" || ext == ".pptx") {
		archive, openErr := zip.OpenReader(snapshot.Name())
		err = openErr
		if err == nil {
			var total uint64
			for _, entry := range archive.File {
				if entry.UncompressedSize64 > maxExpandedBytes-total {
					err = fmt.Errorf("document archive exceeds expanded size limit")
					break
				}
				total += entry.UncompressedSize64
			}
			archive.Close()
		}
	}
	if err != nil {
		cleanup()
		return "", nil, nil, err
	}
	return snapshot.Name(), info, cleanup, nil
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}

func documentReader(ext string) docreader.DocumentReader {
	switch strings.ToLower(ext) {
	case ".docx":
		return &docreader.DocxReader{}
	case ".pdf":
		return &docreader.PdfReader{}
	case ".xlsx":
		return &docreader.XlsxReader{}
	case ".pptx":
		return &docreader.PptxReader{}
	case ".txt":
		return &docreader.TxtReader{}
	case ".csv":
		return &docreader.CsvReader{}
	case ".md", ".markdown":
		return &docreader.MdReader{}
	case ".rtf":
		return &docreader.RtfReader{}
	}
	return nil
}

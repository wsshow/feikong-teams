package fkfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/spf13/afero"
)

// LocalBackend 基于本地文件系统的 filesystem.Backend 实现
type LocalBackend struct {
	fs      afero.Fs
	baseDir string
}

func NewLocalBackend(baseDir string) (*LocalBackend, error) {
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
	}
	return &LocalBackend{
		fs:      afero.NewBasePathFs(afero.NewOsFs(), absPath),
		baseDir: absPath,
	}, nil
}

// resolvePath 将路径转换为相对于 baseDir 的路径，供 BasePathFs 使用
func (b *LocalBackend) resolvePath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(b.baseDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

func (b *LocalBackend) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	path := req.Path
	if path == "" {
		path = "."
	}
	path = b.resolvePath(path)

	entries, err := afero.ReadDir(b.fs, path)
	if err != nil {
		return nil, fmt.Errorf("无法读取目录 %s: %w", path, err)
	}

	result := make([]filesystem.FileInfo, 0, len(entries))
	for _, entry := range entries {
		result = append(result, filesystem.FileInfo{
			Path:       entry.Name(),
			IsDir:      entry.IsDir(),
			Size:       entry.Size(),
			ModifiedAt: entry.ModTime().Format(time.RFC3339),
		})
	}
	return result, nil
}

func (b *LocalBackend) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	filePath := b.resolvePath(req.FilePath)
	data, err := afero.ReadFile(b.fs, filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败 %s: %w", req.FilePath, err)
	}

	content := string(data)

	offset := req.Offset - 1
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 2000
	}

	if offset == 0 {
		lineCount := strings.Count(content, "\n") + 1
		if lineCount <= limit {
			return &filesystem.FileContent{Content: content}, nil
		}
	}

	start := 0
	for i := 0; i < offset; i++ {
		idx := strings.IndexByte(content[start:], '\n')
		if idx == -1 {
			return &filesystem.FileContent{}, nil
		}
		start += idx + 1
	}

	end := start
	for i := 0; i < limit; i++ {
		idx := strings.IndexByte(content[end:], '\n')
		if idx == -1 {
			return &filesystem.FileContent{Content: content[start:]}, nil
		}
		end += idx + 1
	}

	return &filesystem.FileContent{Content: content[start : end-1]}, nil
}

func (b *LocalBackend) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern 不能为空")
	}

	pattern := req.Pattern
	if req.CaseInsensitive {
		pattern = "(?i)" + pattern
	}
	if req.EnableMultiline {
		pattern = "(?s)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("无效的正则表达式: %w", err)
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}
	searchPath = b.resolvePath(searchPath)

	var matches []filesystem.GrepMatch

	err = afero.Walk(b.fs, searchPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		if req.FileType != "" {
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			if !matchFileType(ext, req.FileType) {
				return nil
			}
		}

		if req.Glob != "" {
			var matchPath string
			if strings.Contains(req.Glob, "/") || strings.Contains(req.Glob, "**") {
				matchPath = path
			} else {
				matchPath = filepath.Base(path)
			}
			matched, matchErr := doublestar.Match(req.Glob, matchPath)
			if matchErr != nil || !matched {
				return nil
			}
		}

		data, readErr := afero.ReadFile(b.fs, path)
		if readErr != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, filesystem.GrepMatch{
					Path:    path,
					Line:    i + 1,
					Content: line,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("搜索文件失败: %w", err)
	}

	if req.BeforeLines > 0 || req.AfterLines > 0 {
		matches = b.applyContext(matches, req)
	}

	return matches, nil
}

func (b *LocalBackend) applyContext(matches []filesystem.GrepMatch, req *filesystem.GrepRequest) []filesystem.GrepMatch {
	if len(matches) == 0 {
		return matches
	}

	beforeLines := req.BeforeLines
	afterLines := req.AfterLines
	if beforeLines <= 0 {
		beforeLines = 0
	}
	if afterLines <= 0 {
		afterLines = 0
	}
	if beforeLines == 0 && afterLines == 0 {
		return matches
	}

	matchesByFile := make(map[string][]filesystem.GrepMatch)
	var fileOrder []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m.Path] {
			fileOrder = append(fileOrder, m.Path)
			seen[m.Path] = true
		}
		matchesByFile[m.Path] = append(matchesByFile[m.Path], m)
	}

	var result []filesystem.GrepMatch
	for _, filePath := range fileOrder {
		data, err := afero.ReadFile(b.fs, filePath)
		if err != nil {
			result = append(result, matchesByFile[filePath]...)
			continue
		}
		lines := strings.Split(string(data), "\n")
		processedLines := make(map[int]bool)
		for _, m := range matchesByFile[filePath] {
			startLine := m.Line - beforeLines
			if startLine < 1 {
				startLine = 1
			}
			endLine := m.Line + afterLines
			if endLine > len(lines) {
				endLine = len(lines)
			}
			for lineNum := startLine; lineNum <= endLine; lineNum++ {
				if !processedLines[lineNum] {
					processedLines[lineNum] = true
					result = append(result, filesystem.GrepMatch{
						Path:    filePath,
						Line:    lineNum,
						Content: lines[lineNum-1],
					})
				}
			}
		}
	}
	return result
}

func (b *LocalBackend) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	basePath := req.Path
	if basePath == "" {
		basePath = "."
	}
	basePath = b.resolvePath(basePath)

	var result []filesystem.FileInfo

	err := afero.Walk(b.fs, basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		var matchPath string
		if strings.HasPrefix(req.Pattern, "/") {
			matchPath = path
		} else {
			rel, relErr := filepath.Rel(basePath, path)
			if relErr != nil {
				return nil
			}
			matchPath = rel
		}

		matched, matchErr := doublestar.Match(req.Pattern, matchPath)
		if matchErr != nil {
			return fmt.Errorf("无效的 glob 模式: %w", matchErr)
		}
		if matched {
			result = append(result, filesystem.FileInfo{
				Path:       matchPath,
				IsDir:      false,
				Size:       info.Size(),
				ModifiedAt: info.ModTime().Format(time.RFC3339),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (b *LocalBackend) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	filePath := b.resolvePath(req.FilePath)
	dir := filepath.Dir(filePath)
	if dir != "." {
		if err := b.fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}
	return afero.WriteFile(b.fs, filePath, []byte(req.Content), 0644)
}

func (b *LocalBackend) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	filePath := b.resolvePath(req.FilePath)
	data, err := afero.ReadFile(b.fs, filePath)
	if err != nil {
		return fmt.Errorf("文件不存在: %s", req.FilePath)
	}

	if req.OldString == "" {
		return fmt.Errorf("oldString 不能为空")
	}

	content := string(data)
	if !strings.Contains(content, req.OldString) {
		return fmt.Errorf("在文件 %s 中未找到 oldString", req.FilePath)
	}

	if !req.ReplaceAll {
		firstIdx := strings.Index(content, req.OldString)
		if strings.Contains(content[firstIdx+len(req.OldString):], req.OldString) {
			return fmt.Errorf("在文件 %s 中发现多处匹配，但 ReplaceAll 为 false", req.FilePath)
		}
	}

	var newContent string
	if req.ReplaceAll {
		newContent = strings.ReplaceAll(content, req.OldString, req.NewString)
	} else {
		newContent = strings.Replace(content, req.OldString, req.NewString, 1)
	}

	return afero.WriteFile(b.fs, filePath, []byte(newContent), 0644)
}

func matchFileType(ext, fileType string) bool {
	typeMap := map[string][]string{
		"go":         {"go"},
		"py":         {"py", "pyi"},
		"python":     {"py", "pyi"},
		"js":         {"cjs", "js", "jsx", "mjs", "vue"},
		"ts":         {"cts", "mts", "ts", "tsx"},
		"typescript": {"cts", "mts", "ts", "tsx"},
		"c":          {"c", "h"},
		"cpp":        {"cc", "cpp", "cxx", "h", "hh", "hpp", "hxx"},
		"java":       {"java", "jsp"},
		"rust":       {"rs"},
		"ruby":       {"rb", "gemspec"},
		"sh":         {"bash", "sh", "zsh"},
		"html":       {"htm", "html"},
		"css":        {"css", "scss", "sass", "less"},
		"json":       {"json"},
		"yaml":       {"yaml", "yml"},
		"xml":        {"xml", "xsd", "xsl"},
		"sql":        {"sql"},
		"md":         {"md", "markdown", "mdx"},
		"markdown":   {"md", "markdown", "mdx"},
		"toml":       {"toml"},
		"swift":      {"swift"},
		"kotlin":     {"kt", "kts"},
		"dart":       {"dart"},
		"lua":        {"lua"},
		"php":        {"php"},
		"scala":      {"scala", "sbt"},
	}
	if exts, ok := typeMap[fileType]; ok {
		for _, e := range exts {
			if ext == e {
				return true
			}
		}
	}
	return ext == fileType
}

var _ filesystem.Backend = (*LocalBackend)(nil)

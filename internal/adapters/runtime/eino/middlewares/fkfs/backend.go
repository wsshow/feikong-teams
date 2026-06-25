package fkfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fkteams/common/pathguard"

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
		return nil, fmt.Errorf("resolve absolute path: %w", err)
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Errorf("create directory %s: %w", absPath, err)
	}
	return &LocalBackend{
		fs:      afero.NewBasePathFs(afero.NewOsFs(), absPath),
		baseDir: absPath,
	}, nil
}

// resolvePath 将宿主机绝对路径和虚拟根路径统一约束到 baseDir 下。
func (b *LocalBackend) resolvePath(path string) (string, error) {
	resolved, err := pathguard.ResolveWorkspace(b.baseDir, b.normalizePath(path))
	if err != nil {
		return "", err
	}
	if resolved.RelPath == "" {
		return ".", nil
	}
	return resolved.RelPath, nil
}

func (b *LocalBackend) normalizePath(path string) string {
	if path == "" {
		return "."
	}
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) || isWithinBase(cleanPath, b.baseDir) {
		return cleanPath
	}
	volume := filepath.VolumeName(cleanPath)
	cleanPath = strings.TrimPrefix(cleanPath, volume)
	cleanPath = strings.TrimLeft(cleanPath, string(filepath.Separator))
	if cleanPath == "" {
		return "."
	}
	return cleanPath
}

func isWithinBase(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	return path == base || strings.HasPrefix(path, base+string(os.PathSeparator))
}

func (b *LocalBackend) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("ls request is nil")
	}
	path := req.Path
	if path == "" {
		path = "."
	}
	path, err := b.resolvePath(path)
	if err != nil {
		return nil, err
	}

	entries, err := afero.ReadDir(b.fs, path)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", req.Path, err)
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("read request is nil")
	}
	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return nil, err
	}
	data, err := afero.ReadFile(b.fs, filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", req.FilePath, err)
	}

	content := string(data)

	offset := req.Offset - 1
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit

	if offset == 0 && limit <= 0 {
		return &filesystem.FileContent{Content: content}, nil
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

	if limit <= 0 {
		return &filesystem.FileContent{Content: content[start:]}, nil
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("grep request is nil")
	}
	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern cannot be empty")
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
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}
	searchPath, err = b.resolvePath(searchPath)
	if err != nil {
		return nil, err
	}

	var matches []filesystem.GrepMatch

	err = afero.Walk(b.fs, searchPath, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		return nil, fmt.Errorf("grep files: %w", err)
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("glob request is nil")
	}
	basePath := req.Path
	if basePath == "" {
		basePath = "."
	}
	basePath, err := b.resolvePath(basePath)
	if err != nil {
		return nil, err
	}

	var result []filesystem.FileInfo
	isAbsolutePattern := strings.HasPrefix(req.Pattern, "/")

	err = afero.Walk(b.fs, basePath, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		var matchPath string
		var resultPath string
		if isAbsolutePattern {
			matchPath = "/" + filepath.ToSlash(path)
			resultPath = matchPath
		} else {
			rel, relErr := filepath.Rel(basePath, path)
			if relErr != nil {
				return nil
			}
			matchPath = filepath.ToSlash(rel)
			resultPath = rel
		}

		matched, matchErr := doublestar.Match(req.Pattern, matchPath)
		if matchErr != nil {
			return fmt.Errorf("invalid glob pattern: %w", matchErr)
		}
		if matched {
			result = append(result, filesystem.FileInfo{
				Path:       resultPath,
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("write request is nil")
	}
	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(filePath)
	if dir != "." {
		if err := b.fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	}
	return afero.WriteFile(b.fs, filePath, []byte(req.Content), 0644)
}

func (b *LocalBackend) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("edit request is nil")
	}
	filePath, err := b.resolvePath(req.FilePath)
	if err != nil {
		return err
	}
	data, err := afero.ReadFile(b.fs, filePath)
	if err != nil {
		return fmt.Errorf("read file %s: %w", req.FilePath, err)
	}

	if req.OldString == "" {
		return fmt.Errorf("oldString cannot be empty")
	}
	if req.OldString == req.NewString {
		return fmt.Errorf("oldString and newString must be different")
	}

	content := string(data)
	if !strings.Contains(content, req.OldString) {
		return fmt.Errorf("oldString not found in file %s", req.FilePath)
	}

	if !req.ReplaceAll {
		firstIdx := strings.Index(content, req.OldString)
		if strings.Contains(content[firstIdx+len(req.OldString):], req.OldString) {
			return fmt.Errorf("oldString appears multiple times in file %s; set ReplaceAll to true", req.FilePath)
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

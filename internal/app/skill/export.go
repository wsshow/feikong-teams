package skill

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"fkteams/internal/app/appdata"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"

	"github.com/goccy/go-yaml"
)

var localSkillSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

const (
	maxSkillManifestBytes     int64 = 1 << 20
	maxSkillEditableFileBytes int64 = 16 << 20
	maxSkillDirectoryEntries        = 10_000
)

// LocalSkillInfo 表示本地已安装技能。
type LocalSkillInfo struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LocalSkillSpec 表示本地技能创建请求。
type LocalSkillSpec struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// ListLocalSkills 列出本地已安装技能。
func ListLocalSkills() ([]LocalSkillInfo, error) {
	skillsDir := appdata.SkillsDir()

	directory, err := os.Open(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}
	defer directory.Close()
	entries, err := readSkillDirectoryEntries(directory)
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}
	root, err := os.OpenRoot(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()

	var skills []LocalSkillInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(entry.Name(), "SKILL.md")
		data, err := readSkillRootFile(root, skillFile, maxSkillManifestBytes)
		if err != nil {
			continue
		}

		content := string(data)
		parts := strings.SplitN(content, "---", 3)
		if len(parts) < 3 {
			continue
		}

		var info struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &info); err != nil {
			continue
		}

		name := info.Name
		if name == "" {
			name = entry.Name()
		}

		skills = append(skills, LocalSkillInfo{
			Slug:        entry.Name(),
			Name:        name,
			Description: strings.Join(strings.Fields(info.Description), " "),
		})
	}

	return skills, nil
}

// SkillFileEntry 表示技能目录中的文件。
type SkillFileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ListSkillFiles 列出技能目录下指定路径的文件。
func ListSkillFiles(slug, subPath string) ([]SkillFileEntry, error) {
	_, cleanSub, err := resolveSkillPath(slug, subPath, true)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill %s is not installed", slug)
		}
		return nil, err
	}
	defer root.Close()
	relativeDir := skillRelativePath(slug, cleanSub)
	if err := pathguard.RejectRootSymlinkComponents(root, relativeDir); err != nil {
		return nil, fmt.Errorf("open skill directory: %w", err)
	}
	directory, err := root.Open(relativeDir)
	if err != nil {
		return nil, fmt.Errorf("open skill directory: %w", err)
	}
	entries, err := readSkillDirectoryEntries(directory)
	closeErr := directory.Close()
	if err != nil {
		return nil, fmt.Errorf("read skill directory: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close skill directory: %w", closeErr)
	}

	type sortableSkillFileEntry struct {
		entry       SkillFileEntry
		modUnixNano int64
	}
	var sortableEntries []sortableSkillFileEntry
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		sortableEntries = append(sortableEntries, sortableSkillFileEntry{
			entry: SkillFileEntry{
				Name:  e.Name(),
				Path:  filepath.ToSlash(filepath.Join(cleanSub, e.Name())),
				IsDir: e.IsDir(),
				Size:  info.Size(),
			},
			modUnixNano: info.ModTime().UnixNano(),
		})
	}
	sort.SliceStable(sortableEntries, func(i, j int) bool {
		left := sortableEntries[i]
		right := sortableEntries[j]
		if left.entry.IsDir != right.entry.IsDir {
			return left.entry.IsDir
		}
		if left.modUnixNano != right.modUnixNano {
			return left.modUnixNano > right.modUnixNano
		}
		if left.entry.Size != right.entry.Size {
			return left.entry.Size > right.entry.Size
		}
		return left.entry.Name < right.entry.Name
	})

	result := make([]SkillFileEntry, 0, len(sortableEntries))
	for _, item := range sortableEntries {
		result = append(result, item.entry)
	}
	return result, nil
}

// ReadSkillFile 读取技能目录中的文件。
func ReadSkillFile(slug, filePath string) (string, error) {
	_, cleanSub, err := resolveSkillPath(slug, filePath, false)
	if err != nil {
		return "", err
	}

	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		return "", fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	data, err := readSkillRootFile(root, skillRelativePath(slug, cleanSub), maxSkillEditableFileBytes)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CreateLocalSkill 创建用户自定义本地技能。
func CreateLocalSkill(spec LocalSkillSpec) (LocalSkillInfo, error) {
	spec.Slug = strings.TrimSpace(spec.Slug)
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)
	if err := validateSkillSlug(spec.Slug); err != nil {
		return LocalSkillInfo{}, err
	}
	if spec.Name == "" {
		spec.Name = spec.Slug
	}

	content := strings.TrimSpace(spec.Content)
	if content == "" {
		content = defaultSkillContent(spec.Name, spec.Description)
	}
	data := []byte(content + "\n")
	if int64(len(data)) > maxSkillEditableFileBytes {
		return LocalSkillInfo{}, fmt.Errorf("skill file exceeds %d bytes", maxSkillEditableFileBytes)
	}
	if err := os.MkdirAll(appdata.SkillsDir(), 0755); err != nil {
		return LocalSkillInfo{}, fmt.Errorf("create skills directory: %w", err)
	}
	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		return LocalSkillInfo{}, fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(spec.Slug); err == nil {
		return LocalSkillInfo{}, fmt.Errorf("skill already exists")
	} else if !os.IsNotExist(err) {
		return LocalSkillInfo{}, fmt.Errorf("stat skill dir: %w", err)
	}
	if err := root.Mkdir(spec.Slug, 0755); err != nil {
		return LocalSkillInfo{}, fmt.Errorf("create skill dir: %w", err)
	}
	if err := atomicfile.WriteFileInRoot(root, filepath.Join(spec.Slug, "SKILL.md"), data, 0644); err != nil {
		_ = root.RemoveAll(spec.Slug)
		return LocalSkillInfo{}, err
	}
	return LocalSkillInfo{Slug: spec.Slug, Name: spec.Name, Description: spec.Description}, nil
}

// SaveSkillFile 保存技能文件内容。
func SaveSkillFile(slug, filePath, content string) error {
	_, cleanSub, err := resolveSkillPath(slug, filePath, false)
	if err != nil {
		return err
	}
	if int64(len(content)) > maxSkillEditableFileBytes {
		return fmt.Errorf("skill file exceeds %d bytes", maxSkillEditableFileBytes)
	}
	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		return fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	relativePath := skillRelativePath(slug, cleanSub)
	if err := pathguard.EnsureRootDirectory(root, filepath.Dir(relativePath), 0755); err != nil {
		return fmt.Errorf("prepare skill parent directory: %w", err)
	}
	info, err := root.Lstat(relativePath)
	if err == nil && info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links are not allowed")
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat skill file: %w", err)
	}
	return atomicfile.WriteFileInRoot(root, relativePath, []byte(content), 0644)
}

// CreateSkillFile 创建技能文件或目录。
func CreateSkillFile(slug, filePath, content string, isDir bool) error {
	_, cleanSub, err := resolveSkillPath(slug, filePath, false)
	if err != nil {
		return err
	}
	if int64(len(content)) > maxSkillEditableFileBytes {
		return fmt.Errorf("skill file exceeds %d bytes", maxSkillEditableFileBytes)
	}
	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		return fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	relativePath := skillRelativePath(slug, cleanSub)
	if _, err := root.Lstat(relativePath); err == nil {
		return fmt.Errorf("path already exists")
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat skill path: %w", err)
	}
	if isDir {
		return pathguard.EnsureRootDirectory(root, relativePath, 0755)
	}
	if err := pathguard.EnsureRootDirectory(root, filepath.Dir(relativePath), 0755); err != nil {
		return fmt.Errorf("prepare skill parent directory: %w", err)
	}
	return atomicfile.WriteFileInRoot(root, relativePath, []byte(content), 0644)
}

// DeleteSkillFile 删除技能中的文件或目录。
func DeleteSkillFile(slug, filePath string) error {
	_, cleanSub, err := resolveSkillPath(slug, filePath, false)
	if err != nil {
		return err
	}
	if cleanSub == "SKILL.md" {
		return fmt.Errorf("SKILL.md cannot be deleted")
	}
	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		return fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	relativePath := skillRelativePath(slug, cleanSub)
	if err := pathguard.RejectRootSymlinkComponents(root, relativePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("path not found")
		}
		return fmt.Errorf("inspect skill path: %w", err)
	}
	if _, err := root.Stat(relativePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path not found")
		}
		return fmt.Errorf("stat skill path: %w", err)
	}
	return root.RemoveAll(relativePath)
}

// InstallSkillFromProvider 从指定 provider 安装技能。
func InstallSkillFromProvider(ctx context.Context, slug, version string, provider Provider) error {
	return installSkill(ctx, slug, version, provider)
}

// RemoveLocalSkill 删除已安装技能。
func RemoveLocalSkill(slug string) error {
	if err := validateSkillSlug(slug); err != nil {
		return err
	}
	root, err := os.OpenRoot(appdata.SkillsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill %s is not installed", slug)
		}
		return fmt.Errorf("open skills root: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(slug); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill %s is not installed", slug)
		}
		return fmt.Errorf("stat skill: %w", err)
	}
	return root.RemoveAll(slug)
}

func validateSkillSlug(slug string) error {
	if !localSkillSlugPattern.MatchString(slug) {
		return fmt.Errorf("invalid skill slug")
	}
	return nil
}

func skillRelativePath(slug, cleanSub string) string {
	if cleanSub == "" {
		return slug
	}
	return filepath.Join(slug, filepath.FromSlash(cleanSub))
}

func readSkillDirectoryEntries(directory *os.File) ([]os.DirEntry, error) {
	entries, err := directory.ReadDir(maxSkillDirectoryEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > maxSkillDirectoryEntries {
		return nil, fmt.Errorf("skill directory exceeds %d entries", maxSkillDirectoryEntries)
	}
	return entries, nil
}

func readSkillRootFile(root *os.Root, relativePath string, limit int64) ([]byte, error) {
	if err := pathguard.RejectRootSymlinkComponents(root, relativePath); err != nil {
		return nil, fmt.Errorf("inspect skill file: %w", err)
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return nil, fmt.Errorf("open skill file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat skill file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("skill path is not a regular file")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read skill file: %w", readErr)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("skill file exceeds %d bytes", limit)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close skill file: %w", closeErr)
	}
	return data, nil
}

func resolveSkillPath(slug, subPath string, allowRoot bool) (string, string, error) {
	if err := validateSkillSlug(slug); err != nil {
		return "", "", err
	}
	skillsDir := filepath.Join(appdata.SkillsDir(), slug)
	cleanSub := filepath.Clean(filepath.ToSlash(strings.TrimSpace(subPath)))
	if cleanSub == "." {
		cleanSub = ""
	}
	if cleanSub == "" && !allowRoot {
		return "", "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(cleanSub, "..") || strings.HasPrefix(cleanSub, "/") || filepath.IsAbs(cleanSub) {
		return "", "", fmt.Errorf("invalid path")
	}

	targetPath := filepath.Join(skillsDir, filepath.FromSlash(cleanSub))
	cleanTarget := filepath.Clean(targetPath)
	cleanRoot := filepath.Clean(skillsDir)
	if cleanTarget != cleanRoot && !strings.HasPrefix(cleanTarget, cleanRoot+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("invalid path")
	}
	return cleanTarget, cleanSub, nil
}

func defaultSkillContent(name, description string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nname: %s\ndescription: %s\n---\n\n", yamlQuote(name), yamlQuote(description))
	fmt.Fprintf(&sb, "# %s\n\n", name)
	if description != "" {
		fmt.Fprintf(&sb, "%s\n\n", description)
	}
	sb.WriteString("## Use when\n\n- Describe when this skill should be used.\n\n")
	sb.WriteString("## Instructions\n\n- Add the reusable workflow, constraints, and examples here.\n")
	return sb.String()
}

func yamlQuote(value string) string {
	data, err := yaml.Marshal(value)
	if err != nil {
		return `""`
	}
	return strings.TrimSpace(string(data))
}

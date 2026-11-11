package memory

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domainmemory "fkteams/internal/domain/memory"
	"fkteams/internal/runtime/atomicfile"
)

const (
	timeLayout             = "2006-01-02 15:04:05"
	maxMemoryMarkdownBytes = 4 << 20
	maxMemoryLineBytes     = 256 << 10
)

// memoryTypeFile 记忆类型到文件名和标题的映射
var memoryTypeFile = []struct {
	Type  domainmemory.MemoryType
	File  string
	Title string
}{
	{domainmemory.Preference, "preference.md", "用户偏好"},
	{domainmemory.Fact, "fact.md", "个人信息"},
	{domainmemory.Feedback, "feedback.md", "行为反馈"},
	{domainmemory.Lesson, "lesson.md", "避坑记录"},
	{domainmemory.Decision, "decision.md", "已确定方案"},
	{domainmemory.Insight, "insight.md", "认知洞察"},
	{domainmemory.Experience, "experience.md", "操作经验"},
}

// saveAllMarkdown 按类型保存到多个 Markdown 文件
func saveAllMarkdown(dir string, entries []domainmemory.MemoryEntry) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 按类型分组
	grouped := make(map[domainmemory.MemoryType][]domainmemory.MemoryEntry)
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	for _, tf := range memoryTypeFile {
		path := filepath.Join(dir, tf.File)
		items := grouped[tf.Type]
		if len(items) == 0 {
			if err := removeMemoryFile(path); err != nil {
				return fmt.Errorf("remove empty %s: %w", tf.File, err)
			}
			continue
		}
		if err := writeMarkdownFile(path, tf.Title, items); err != nil {
			return fmt.Errorf("failed to save %s: %w", tf.File, err)
		}
	}
	// 生成 MEMORY.md 入口索引
	if err := writeIndexFile(dir, entries); err != nil {
		return fmt.Errorf("failed to save MEMORY.md: %w", err)
	}
	return nil
}

// writeMarkdownFile 写入单个类型的 Markdown 文件
func writeMarkdownFile(path, title string, entries []domainmemory.MemoryEntry) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", title)

	for _, e := range entries {
		sb.WriteString("## ")
		sb.WriteString(e.Summary)
		sb.WriteString("\n\n")
		sb.WriteString("- 详情: ")
		sb.WriteString(e.Detail)
		sb.WriteString("\n")
		sb.WriteString("- 标签: ")
		sb.WriteString(strings.Join(e.Tags, ", "))
		sb.WriteString("\n")
		sb.WriteString("- 创建: ")
		sb.WriteString(e.CreatedAt.Format(timeLayout))
		sb.WriteString("\n")
		sb.WriteString("- 命中: ")
		sb.WriteString(strconv.Itoa(e.HitCount))
		if e.LastHitAt != nil {
			sb.WriteString(" | 最后命中: ")
			sb.WriteString(e.LastHitAt.Format(timeLayout))
		}
		sb.WriteString("\n\n")
	}

	return atomicfile.WriteFile(path, []byte(sb.String()), 0644)
}

// loadAllMarkdown 从目录加载所有类型的 Markdown 文件
func loadAllMarkdown(dir string, maxEntries int) ([]domainmemory.MemoryEntry, error) {
	if maxEntries <= 0 {
		return nil, nil
	}
	var all []domainmemory.MemoryEntry
	for _, tf := range memoryTypeFile {
		remaining := maxEntries - len(all)
		if remaining <= 0 {
			break
		}
		path := filepath.Join(dir, tf.File)
		entries, err := loadMarkdownFile(path, tf.Type, remaining)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("load %s: %w", tf.File, err)
			}
			continue
		}
		all = append(all, entries...)
	}
	return all, nil
}

// loadMarkdownFile 从单个 Markdown 文件加载记忆条目
func loadMarkdownFile(path string, memType domainmemory.MemoryType, maxEntries int) ([]domainmemory.MemoryEntry, error) {
	if maxEntries <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxMemoryMarkdownBytes {
		return nil, fmt.Errorf("memory file exceeds %d bytes", maxMemoryMarkdownBytes)
	}

	var entries []domainmemory.MemoryEntry
	var current *domainmemory.MemoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64<<10), maxMemoryLineBytes)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				entries = append(entries, *current)
				if len(entries) >= maxEntries {
					current = nil
					break
				}
			}
			current = &domainmemory.MemoryEntry{
				Summary: strings.TrimSpace(strings.TrimPrefix(line, "## ")),
				Type:    memType,
			}
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "- 详情: ") {
			current.Detail = strings.TrimPrefix(line, "- 详情: ")
		} else if strings.HasPrefix(line, "- 标签: ") {
			tagsStr := strings.TrimPrefix(line, "- 标签: ")
			if tagsStr != "" {
				tags := strings.Split(tagsStr, ", ")
				for i := range tags {
					tags[i] = strings.TrimSpace(tags[i])
				}
				current.Tags = tags
			}
		} else if strings.HasPrefix(line, "- 创建: ") {
			if t, err := time.Parse(timeLayout, strings.TrimPrefix(line, "- 创建: ")); err == nil {
				current.CreatedAt = t
			}
		} else if strings.HasPrefix(line, "- 命中: ") {
			hitStr := strings.TrimPrefix(line, "- 命中: ")
			parts := strings.SplitN(hitStr, " | 最后命中: ", 2)
			if count, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				current.HitCount = count
			}
			if len(parts) == 2 {
				if t, err := time.Parse(timeLayout, strings.TrimSpace(parts[1])); err == nil {
					current.LastHitAt = &t
				}
			}
		}
	}

	if current != nil && len(entries) < maxEntries {
		entries = append(entries, *current)
	}

	// 生成唯一 ID（同类型同秒的条目通过索引区分）
	for i := range entries {
		entries[i].ID = fmt.Sprintf("%s_%d_%d", entries[i].Type, entries[i].CreatedAt.Unix(), i)
	}

	return entries, scanner.Err()
}

// writeIndexFile 生成 MEMORY.md 入口索引，人类可读
func writeIndexFile(dir string, entries []domainmemory.MemoryEntry) error {
	if len(entries) == 0 {
		return removeMemoryFile(filepath.Join(dir, "MEMORY.md"))
	}

	grouped := make(map[domainmemory.MemoryType][]domainmemory.MemoryEntry)
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	var sb strings.Builder
	sb.WriteString("# 记忆索引\n\n")
	fmt.Fprintf(&sb, "共 %d 条记忆，最后更新: %s\n\n", len(entries), time.Now().Format(timeLayout))

	for _, tf := range memoryTypeFile {
		items := grouped[tf.Type]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "## %s (%d 条)\n\n", tf.Title, len(items))
		for _, e := range items {
			fmt.Fprintf(&sb, "- **%s**：%s\n", e.Summary, e.Detail)
		}
		sb.WriteString("\n")
	}

	return atomicfile.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(sb.String()), 0644)
}

func removeMemoryFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Store 使用 Markdown 文件保存长期记忆。
type Store struct{ dir string }

// NewStore 创建文件存储；实际访问错误由 Load 和 Save 返回。
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Load 读取至多 limit 条记忆。
func (s *Store) Load(limit int) ([]domainmemory.MemoryEntry, error) {
	return loadAllMarkdown(s.dir, limit)
}

// Save 保存完整记忆快照。
func (s *Store) Save(entries []domainmemory.MemoryEntry) error {
	return saveAllMarkdown(s.dir, entries)
}

package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const timeLayout = "2006-01-02 15:04:05"

// memoryTypeFile 记忆类型到文件名和标题的映射
var memoryTypeFile = []struct {
	Type  MemoryType
	File  string
	Title string
}{
	{Preference, "preference.md", "💡 用户偏好"},
	{Fact, "fact.md", "📌 个人信息"},
	{Lesson, "lesson.md", "⚠️ 避坑记录"},
	{Decision, "decision.md", "✅ 已确定方案"},
	{Insight, "insight.md", "🔍 认知洞察"},
}

// saveAllMarkdown 按类型保存到多个 Markdown 文件
func saveAllMarkdown(dir string, entries []MemoryEntry) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 按类型分组
	grouped := make(map[MemoryType][]MemoryEntry)
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	for _, tf := range memoryTypeFile {
		path := filepath.Join(dir, tf.File)
		items := grouped[tf.Type]
		if len(items) == 0 {
			os.Remove(path)
			continue
		}
		if err := writeMarkdownFile(path, tf.Title, items); err != nil {
			return fmt.Errorf("failed to save %s: %w", tf.File, err)
		}
	}
	return nil
}

// writeMarkdownFile 写入单个类型的 Markdown 文件
func writeMarkdownFile(path, title string, entries []MemoryEntry) error {
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

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// loadAllMarkdown 从目录加载所有类型的 Markdown 文件
func loadAllMarkdown(dir string) []MemoryEntry {
	var all []MemoryEntry
	for _, tf := range memoryTypeFile {
		path := filepath.Join(dir, tf.File)
		entries, err := loadMarkdownFile(path, tf.Type)
		if err != nil {
			continue
		}
		all = append(all, entries...)
	}
	return all
}

// loadMarkdownFile 从单个 Markdown 文件加载记忆条目
func loadMarkdownFile(path string, memType MemoryType) ([]MemoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []MemoryEntry
	var current *MemoryEntry
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				entries = append(entries, *current)
			}
			current = &MemoryEntry{
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

	if current != nil {
		entries = append(entries, *current)
	}

	// 为条目生成 ID
	for i := range entries {
		entries[i].ID = fmt.Sprintf("%s_%d", entries[i].Type, entries[i].CreatedAt.UnixNano())
	}

	return entries, scanner.Err()
}

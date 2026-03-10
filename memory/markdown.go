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

// saveMarkdown 将记忆条目保存为 Markdown 格式
func saveMarkdown(path string, entries []MemoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# 长期记忆\n\n")

	for _, e := range entries {
		sb.WriteString("## ")
		sb.WriteString(e.Summary)
		sb.WriteString("\n\n")
		sb.WriteString("- 类型: ")
		sb.WriteString(string(e.Type))
		sb.WriteString("\n")
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

// loadMarkdown 从 Markdown 文件加载记忆条目
func loadMarkdown(path string) ([]MemoryEntry, error) {
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

		// 新条目：以 "## " 开头
		if strings.HasPrefix(line, "## ") {
			if current != nil {
				entries = append(entries, *current)
			}
			summary := strings.TrimPrefix(line, "## ")
			current = &MemoryEntry{
				Summary: strings.TrimSpace(summary),
			}
			continue
		}

		if current == nil {
			continue
		}

		// 解析元数据行
		if strings.HasPrefix(line, "- 类型: ") {
			current.Type = MemoryType(strings.TrimPrefix(line, "- 类型: "))
		} else if strings.HasPrefix(line, "- 详情: ") {
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
			timeStr := strings.TrimPrefix(line, "- 创建: ")
			if t, err := time.Parse(timeLayout, timeStr); err == nil {
				current.CreatedAt = t
			}
		} else if strings.HasPrefix(line, "- 命中: ") {
			hitStr := strings.TrimPrefix(line, "- 命中: ")
			// 可能包含 "| 最后命中: ..."
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

	// 最后一个条目
	if current != nil {
		entries = append(entries, *current)
	}

	// 为没有 ID 的条目生成 ID
	for i := range entries {
		if entries[i].ID == "" {
			entries[i].ID = generateID(entries[i])
		}
		if !AllMemoryTypes[entries[i].Type] {
			entries[i].Type = Fact
		}
	}

	return entries, scanner.Err()
}

// generateID 根据条目内容生成确定性 ID
func generateID(entry MemoryEntry) string {
	return fmt.Sprintf("%s_%d", entry.Type, entry.CreatedAt.UnixNano())
}

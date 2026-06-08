package runtime

import (
	"fkteams/events/log"
	"fkteams/tui"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

// ListSessions 列出所有可用的聊天历史会话，interactive 表示是否在交互模式中调用。
func ListSessions(interactive ...bool) {
	entries, err := os.ReadDir(CLIHistoryDir)
	if err != nil {
		pterm.Error.Printfln("读取历史目录失败: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString("# 可用的聊天历史会话\n\n")
	sb.WriteString("| 会话 ID | 标题 | 修改时间 | 大小 |\n")
	sb.WriteString("|---------|------|----------|------|\n")

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		sessionDir := filepath.Join(CLIHistoryDir, sessionID)

		title := sessionID
		if meta, err := eventlog.LoadMetadata(sessionDir); err == nil {
			title = meta.Title
		}

		historyFile := filepath.Join(sessionDir, eventlog.HistoryFileName)
		if info, err := os.Stat(historyFile); err == nil {
			fmt.Fprintf(&sb, "| `%s` | %s | %s | %d B |\n",
				sessionID, title, info.ModTime().Format("2006-01-02 15:04:05"), info.Size())
		} else {
			fmt.Fprintf(&sb, "| `%s` | %s | - | - |\n", sessionID, title)
		}
		count++
	}

	if count == 0 {
		pterm.Info.Println("暂无聊天历史文件")
		return
	}

	if len(interactive) > 0 && interactive[0] {
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `load_chat_history` 加载\n", count)
	} else {
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `fkteams --resume <session_id>` 恢复会话\n", count)
	}
	sb.WriteString("\n---\n")
	fmt.Println(tui.RenderMarkdown(sb.String()))
}

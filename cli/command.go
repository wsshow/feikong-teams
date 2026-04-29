package cli

import (
	"context"
	"fkteams/agents"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/memory"
	"fkteams/tools/scheduler"
	"fkteams/tui"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
)

// CommandResult 命令执行结果
type CommandResult int

const (
	ResultContinue CommandResult = iota // 继续处理
	ResultHandled                       // 命令已处理，继续循环
	ResultExit                          // 退出程序
	ResultNotFound                      // 命令未找到
)

// CommandHandler 命令处理器
type CommandHandler struct {
	modeSwitcher ModeSwitcher
}

// ModeSwitcher 工作模式切换器接口
type ModeSwitcher interface {
	SwitchMode() (newMode string, err error)
}

// NewCommandHandler 创建命令处理器
func NewCommandHandler(modeSwitcher ModeSwitcher) *CommandHandler {
	return &CommandHandler{
		modeSwitcher: modeSwitcher,
	}
}

// Handle 处理命令，返回命令执行结果
func (h *CommandHandler) Handle(input string) CommandResult {
	switch input {
	case "q", "quit", "":
		return ResultExit

	case "help":
		helpMD := `# fkteams 命令帮助

## 基本操作
| 命令 | 说明 |
|------|------|
| ` + "`help`" + ` | 显示此帮助信息 |
| ` + "`q`" + ` / ` + "`quit`" + ` | 退出程序 |

## 智能体切换
| 命令 | 说明 |
|------|------|
| ` + "`list_agents`" + ` | 列出所有可用的智能体 |
| ` + "`@智能体名 [查询内容]`" + ` | 切换到指定智能体并可选执行查询 |

## 文件引用
| 命令 | 说明 |
|------|------|
| ` + "`#文件路径`" + ` | 快速引用工作目录中的文件或文件夹 |

## 聊天历史管理
| 命令 | 说明 |
|------|------|
| ` + "`list_chat_history`" + ` | 列出所有可用的聊天历史会话 |
| ` + "`load_chat_history`" + ` | 选择并加载聊天历史会话 |
| ` + "`save_chat_history`" + ` | 保存聊天历史到当前会话文件 |
| ` + "`clear_chat_history`" + ` | 清空当前聊天历史 |
| ` + "`save_chat_history_to_markdown`" + ` | 导出聊天历史为 Markdown 文件 |
| ` + "`save_chat_history_to_html`" + ` | 导出聊天历史为 HTML 文件 |

## 任务管理
| 命令 | 说明 |
|------|------|
| ` + "`list_schedule`" + ` | 列出所有定时任务 |
| ` + "`cancel_schedule`" + ` | 选择并取消定时任务 |
| ` + "`delete_schedule`" + ` | 选择并删除定时任务 |

## 模式切换
| 命令 | 说明 |
|------|------|
| ` + "`switch_work_mode`" + ` | 切换当前工作模式 |

## 长期记忆管理
| 命令 | 说明 |
|------|------|
| ` + "`list_memory`" + ` | 列出所有长期记忆条目 |
| ` + "`delete_memory`" + ` | 选择并删除记忆条目 |
| ` + "`clear_memory`" + ` | 清空所有长期记忆 |

## 其他
> 直接输入问题即可与智能体团队对话

---`
		fmt.Println(fkevent.RenderMarkdown(helpMD))
		return ResultHandled

	case "list_chat_history":
		ListSessions(true)
		return ResultHandled

	case "save_chat_history":
		recorder := getCliRecorder()
		historyFile := filepath.Join(CLIHistoryDir, activeSessionID, "history.json")
		err := recorder.SaveToFile(historyFile)
		if err != nil {
			pterm.Error.Printfln("保存聊天历史失败: %v", err)
		} else {
			pterm.Success.Printfln("成功保存聊天历史: %s", historyFile)
			saveCliSessionMetadata(activeSessionID, cliSessionTitle)
		}
		return ResultHandled

	case "clear_chat_history":
		fkevent.GlobalSessionManager.Clear(activeSessionID)
		pterm.Success.Println("成功清空当前聊天历史")
		return ResultHandled

	case "save_chat_history_to_markdown":
		recorder := getCliRecorder()
		filePath, err := recorder.SaveToMarkdownWithTimestamp()
		if err != nil {
			pterm.Error.Printfln("保存聊天历史到 Markdown 失败: %v", err)
		} else {
			pterm.Success.Printfln("成功保存聊天历史到 Markdown 文件: %s", filePath)
		}
		return ResultHandled

	case "switch_work_mode":
		if h.modeSwitcher == nil {
			pterm.Error.Println("模式切换器未配置")
			return ResultHandled
		}
		newMode, err := h.modeSwitcher.SwitchMode()
		if err != nil {
			pterm.Error.Printfln("切换工作模式失败: %v", err)
		} else {
			pterm.Success.Printfln("成功切换到工作模式: %s", newMode)
		}
		return ResultHandled

	case "save_chat_history_to_html":
		htmlFilePath, err := SaveChatHistoryToHTML()
		if err != nil {
			pterm.Error.Printfln("%v", err)
		} else {
			pterm.Success.Printfln("成功保存聊天历史到 HTML 文件: %s", htmlFilePath)
		}
		return ResultHandled

	case "list_agents":
		ListAvailableAgents()
		return ResultHandled

	case "list_memory":
		handleListMemory()
		return ResultHandled

	case "delete_memory":
		handleDeleteMemory()
		return ResultHandled

	case "clear_memory":
		handleClearMemory()
		return ResultHandled

	case "cancel_schedule":
		handleCancelSchedule()
		return ResultHandled

	case "delete_schedule":
		handleDeleteSchedule()
		return ResultHandled

	case "load_chat_history":
		handleLoadSession()
		return ResultHandled

	case "list_schedule":
		s := scheduler.Global()
		if s == nil {
			pterm.Error.Println("定时任务调度器未初始化")
			return ResultHandled
		}
		tasks, err := s.GetTasks("")
		if err != nil {
			pterm.Error.Printfln("获取定时任务列表失败: %v", err)
			return ResultHandled
		}
		pterm.Println()
		pterm.Println(scheduler.FormatTasksForDisplay(tasks))
		return ResultHandled

	default:
		return ResultNotFound
	}
}

// ListSessions 列出所有可用的聊天历史会话，interactive 表示是否在交互模式中调用
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
		if meta, err := fkevent.LoadMetadata(sessionDir); err == nil {
			title = meta.Title
		}

		histFile := filepath.Join(sessionDir, "history.json")
		if info, err := os.Stat(histFile); err == nil {
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
		fmt.Fprintf(&sb, "\n共 **%d** 个会话，使用 `-r <session_id>` 恢复会话\n", count)
	}
	sb.WriteString("\n---\n")
	fmt.Println(fkevent.RenderMarkdown(sb.String()))
}

// loadSession 加载指定 session ID 的聊天历史
func loadSession(sessionID string) {
	historyFile := filepath.Join(CLIHistoryDir, sessionID, "history.json")
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		pterm.Error.Printfln("历史文件不存在: %s", historyFile)
		pterm.Info.Println("使用 list_chat_history 查看可用的会话")
		return
	}

	// 更新当前活跃会话 ID
	activeSessionID = sessionID

	recorder := getCliRecorder()
	err := recorder.LoadFromFile(historyFile)
	if err != nil {
		pterm.Error.Printfln("加载聊天历史失败: %v", err)
	} else {
		pterm.Success.Printfln("成功加载聊天历史，当前会话 ID: %s", sessionID)
	}
}

// ListAvailableAgents 列出所有可用的智能体
func ListAvailableAgents() {
	var sb strings.Builder
	sb.WriteString("# 可用智能体列表\n\n")
	sb.WriteString("> 使用方式: 输入 `@智能体名 [查询内容]` 即可切换到该智能体\n\n")

	for _, agent := range agents.GetRegistry() {
		fmt.Fprintf(&sb, "- **@%s** — %s\n", agent.Name, agent.Description)
	}

	sb.WriteString("\n> 输入 `@` 后会自动提示可用的智能体")
	sb.WriteString("\n---\n")
	fmt.Println(fkevent.RenderMarkdown(sb.String()))
}

// handleListMemory 列出所有长期记忆条目
func handleListMemory() {
	if g.MemoryManager == nil {
		pterm.Error.Println("长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return
	}

	entries := g.MemoryManager.List()
	if len(entries) == 0 {
		pterm.Info.Println("暂无长期记忆条目")
		return
	}

	var sb strings.Builder
	sb.WriteString("# 长期记忆列表\n\n")
	printMemoryEntries(entries, &sb)
	fmt.Fprintf(&sb, "---\n共 **%d** 条记忆，使用 `delete_memory` 删除条目，或 `clear_memory` 清空全部", len(entries))
	fmt.Println(fkevent.RenderMarkdown(sb.String()))
}

// handleDeleteMemory 交互式选择并删除记忆条目
func handleDeleteMemory() {
	if g.MemoryManager == nil {
		pterm.Error.Println("长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return
	}

	entries := g.MemoryManager.List()
	if len(entries) == 0 {
		pterm.Info.Println("暂无记忆条目可删除")
		return
	}

	var items []tui.SelectItem
	for _, e := range entries {
		items = append(items, tui.SelectItem{
			Label: fmt.Sprintf("[%s] %s - %s", e.Type, e.Summary, e.Detail),
			Value: e.Summary,
		})
	}

	selected, err := tui.SelectFromList("选择要删除的记忆", items)
	if err != nil {
		pterm.Warning.Println("已取消删除操作")
		return
	}

	deleted := g.MemoryManager.Delete(selected)
	if deleted > 0 {
		pterm.Success.Printfln("成功删除 %d 条记忆: %s", deleted, selected)
	} else {
		pterm.Warning.Printfln("未找到匹配的记忆条目: %s", selected)
	}
}

// handleCancelSchedule 交互式选择并取消定时任务
func handleCancelSchedule() {
	s := scheduler.Global()
	if s == nil {
		pterm.Error.Println("定时任务调度器未初始化")
		return
	}

	tasks, err := s.GetTasks("pending")
	if err != nil {
		pterm.Error.Printfln("获取定时任务列表失败: %v", err)
		return
	}

	if len(tasks) == 0 {
		pterm.Info.Println("暂无可取消的定时任务")
		return
	}

	var items []tui.SelectItem
	for _, t := range tasks {
		items = append(items, tui.SelectItem{
			Label: fmt.Sprintf("%s - %s (下次: %s)", t.ID, t.Task, t.NextRunAt.Format("2006-01-02 15:04")),
			Value: t.ID,
		})
	}

	selected, err := tui.SelectFromList("选择要取消的定时任务", items)
	if err != nil {
		pterm.Warning.Println("已取消操作")
		return
	}

	ctx := context.Background()
	resp, _ := s.ScheduleCancel(ctx, &scheduler.ScheduleCancelRequest{TaskID: selected})
	if resp.ErrorMessage != "" {
		pterm.Error.Println(resp.ErrorMessage)
	} else {
		pterm.Success.Println(resp.Message)
	}
}

// handleDeleteSchedule 交互式选择并删除定时任务
func handleDeleteSchedule() {
	s := scheduler.Global()
	if s == nil {
		pterm.Error.Println("定时任务调度器未初始化")
		return
	}

	tasks, err := s.GetTasks("")
	if err != nil {
		pterm.Error.Printfln("获取定时任务列表失败: %v", err)
		return
	}

	// 过滤掉 running 状态的任务
	var deletable []scheduler.ScheduledTask
	for _, t := range tasks {
		if t.Status != "running" {
			deletable = append(deletable, t)
		}
	}

	if len(deletable) == 0 {
		pterm.Info.Println("暂无可删除的定时任务")
		return
	}

	var items []tui.SelectItem
	for _, t := range deletable {
		items = append(items, tui.SelectItem{
			Label: fmt.Sprintf("[%s] %s - %s", t.Status, t.ID, t.Task),
			Value: t.ID,
		})
	}

	selected, err := tui.SelectFromList("选择要删除的定时任务", items)
	if err != nil {
		pterm.Warning.Println("已取消操作")
		return
	}

	ctx := context.Background()
	resp, _ := s.ScheduleDelete(ctx, &scheduler.ScheduleDeleteRequest{TaskID: selected})
	if resp.ErrorMessage != "" {
		pterm.Error.Println(resp.ErrorMessage)
	} else {
		pterm.Success.Println(resp.Message)
	}
}

// handleLoadSession 交互式选择并加载聊天历史
func handleLoadSession() {
	historyEntries, err := os.ReadDir(CLIHistoryDir)
	if err != nil {
		pterm.Error.Printfln("读取历史目录失败: %v", err)
		return
	}

	var items []tui.SelectItem
	for _, entry := range historyEntries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		sessionDir := filepath.Join(CLIHistoryDir, sessionID)

		title := sessionID
		if meta, err := fkevent.LoadMetadata(sessionDir); err == nil {
			title = meta.Title
		}

		label := title
		histFile := filepath.Join(sessionDir, "history.json")
		if info, err := os.Stat(histFile); err == nil {
			label = fmt.Sprintf("%s (%s, %d bytes)", title, info.ModTime().Format("2006-01-02 15:04:05"), info.Size())
		}
		items = append(items, tui.SelectItem{Label: label, Value: sessionID})
	}

	if len(items) == 0 {
		pterm.Info.Println("暂无聊天历史文件")
		return
	}

	selected, err := tui.SelectFromList("选择要加载的聊天历史", items)
	if err != nil {
		pterm.Warning.Println("已取消加载操作")
		return
	}

	loadSession(selected)
}

// printMemoryEntries 将记忆条目写入 strings.Builder
func printMemoryEntries(entries []memory.MemoryEntry, sb *strings.Builder) {
	for i, e := range entries {
		fmt.Fprintf(sb, "%d. **[%s]** %s\n", i+1, e.Type, e.Summary)
		fmt.Fprintf(sb, "   %s\n", e.Detail)
		fmt.Fprintf(sb, "   标签: `%s` | 命中: **%d** 次 | 创建: %s\n\n",
			strings.Join(e.Tags, "`, `"), e.HitCount, e.CreatedAt.Format("2006-01-02 15:04"))
	}
}

// handleClearMemory 清空所有长期记忆
func handleClearMemory() {
	if g.MemoryManager == nil {
		pterm.Error.Println("长期记忆未启用，请在 config.toml 中设置 [memory] enabled = true")
		return
	}

	count := g.MemoryManager.Count()
	if count == 0 {
		pterm.Info.Println("当前没有记忆条目")
		return
	}

	g.MemoryManager.Clear()
	pterm.Success.Printfln("成功清空 %d 条长期记忆", count)
}

package cli

import (
	"context"
	"fkteams/agents"
	"fkteams/common"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/memory"
	"fkteams/tools/scheduler"
	"fkteams/tui"
	"fmt"
	"os"
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
		pterm.Println("=== fkteams 命令帮助 ===")
		pterm.Println()
		pterm.Println("基本操作:")
		pterm.Println("  help                             显示此帮助信息")
		pterm.Println("  q, quit, Enter                   退出程序")
		pterm.Println()
		pterm.Println("智能体切换:")
		pterm.Println("  list_agents                     列出所有可用的智能体")
		pterm.Println("  @智能体名 [查询内容]              切换到指定智能体并可选执行查询")
		pterm.Println()
		pterm.Println("文件引用:")
		pterm.Println("  #文件路径                        快速引用工作目录中的文件或文件夹")
		pterm.Println()
		pterm.Println("聊天历史管理:")
		pterm.Println("  list_chat_history               列出所有可用的聊天历史会话")
		pterm.Println("  load_chat_history               选择并加载聊天历史会话")
		pterm.Println("  save_chat_history               保存聊天历史到当前会话文件")
		pterm.Println("  clear_chat_history              清空当前聊天历史")
		pterm.Println("  save_chat_history_to_markdown   导出聊天历史为 Markdown 文件")
		pterm.Println("  save_chat_history_to_html       导出聊天历史为 HTML 文件")
		pterm.Println()
		pterm.Println("任务管理:")
		pterm.Println("  list_schedule                   列出所有定时任务")
		pterm.Println("  cancel_schedule                 选择并取消定时任务")
		pterm.Println()
		pterm.Println("模式切换:")
		pterm.Println("  switch_work_mode               切换当前工作模式")
		pterm.Println()
		pterm.Println("长期记忆管理:")
		pterm.Println("  list_memory                    列出所有长期记忆条目")
		pterm.Println("  delete_memory                  选择并删除记忆条目")
		pterm.Println("  clear_memory                   清空所有长期记忆")
		pterm.Println()
		pterm.Println("其他操作:")
		pterm.Println("  直接输入问题                     与智能体团队对话")
		return ResultHandled

	case "list_chat_history":
		ListChatHistoryFiles(true)
		return ResultHandled

	case "save_chat_history":
		recorder := getCliRecorder()
		historyFile := CLIHistoryDir + common.ChatHistoryPrefix + activeSessionID
		err := recorder.SaveToFile(historyFile)
		if err != nil {
			pterm.Error.Printfln("保存聊天历史失败: %v", err)
		} else {
			pterm.Success.Printfln("成功保存聊天历史: %s", historyFile)
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

	case "load_chat_history":
		handleLoadChatHistory()
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

// ListChatHistoryFiles 列出所有可用的聊天历史文件，interactive 表示是否在交互模式中调用
func ListChatHistoryFiles(interactive ...bool) {
	entries, err := os.ReadDir(CLIHistoryDir)
	if err != nil {
		pterm.Error.Printfln("读取历史目录失败: %v", err)
		return
	}

	pterm.Println()
	pterm.Println("=== 可用的聊天历史会话 ===")
	pterm.Println()

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, common.ChatHistoryPrefix) {
			continue
		}
		sessionID := strings.TrimPrefix(name, common.ChatHistoryPrefix)
		info, _ := entry.Info()
		if info != nil {
			pterm.Printf("  %s  (%s, %d bytes)\n", sessionID, info.ModTime().Format("2006-01-02 15:04:05"), info.Size())
		} else {
			pterm.Printf("  %s\n", sessionID)
		}
		count++
	}

	if count == 0 {
		pterm.Info.Println("暂无聊天历史文件")
	} else {
		pterm.Println()
		if len(interactive) > 0 && interactive[0] {
			pterm.Printf("共 %d 个会话，使用 load_chat_history <session_id> 加载\n", count)
		} else {
			pterm.Printf("共 %d 个会话，使用 -r <session_id> 恢复会话\n", count)
		}
	}
	pterm.Println()
}

// loadChatHistory 加载指定 session ID 的聊天历史
func loadChatHistory(sessionID string) {
	historyFile := CLIHistoryDir + common.ChatHistoryPrefix + sessionID
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
	pterm.Println()
	pterm.Println("=== 可用智能体列表 ===")
	pterm.Println()
	pterm.Println("使用方式: 输入 @智能体名 [查询内容] 即可切换到该智能体")
	pterm.Println()

	for _, agent := range agents.GetRegistry() {
		pterm.Printf("  @%s\n", agent.Name)
		pterm.Printf("    描述: %s\n", agent.Description)
		pterm.Println()
	}

	pterm.Println("提示: 输入 @ 后会自动提示可用的智能体")
	pterm.Println()
}

// handleListMemory 列出所有长期记忆条目
func handleListMemory() {
	if g.MemoryManager == nil {
		pterm.Error.Println("长期记忆未启用，请设置 FEIKONG_MEMORY_ENABLED=true")
		return
	}

	entries := g.MemoryManager.List()
	if len(entries) == 0 {
		pterm.Info.Println("暂无长期记忆条目")
		return
	}

	pterm.Println()
	pterm.Println("=== 长期记忆列表 ===")
	pterm.Println()

	printMemoryEntries(entries)

	pterm.Printf("共 %d 条记忆\n", len(entries))
	pterm.Printf("使用 delete_memory 选择删除条目，或 clear_memory 清空全部\n")
	pterm.Println()
}

// handleDeleteMemory 交互式选择并删除记忆条目
func handleDeleteMemory() {
	if g.MemoryManager == nil {
		pterm.Error.Println("长期记忆未启用，请设置 FEIKONG_MEMORY_ENABLED=true")
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

// handleLoadChatHistory 交互式选择并加载聊天历史
func handleLoadChatHistory() {
	historyEntries, err := os.ReadDir(CLIHistoryDir)
	if err != nil {
		pterm.Error.Printfln("读取历史目录失败: %v", err)
		return
	}

	var items []tui.SelectItem
	for _, entry := range historyEntries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), common.ChatHistoryPrefix) {
			continue
		}
		sessionID := strings.TrimPrefix(entry.Name(), common.ChatHistoryPrefix)
		label := sessionID
		if info, _ := entry.Info(); info != nil {
			label = fmt.Sprintf("%s (%s, %d bytes)", sessionID, info.ModTime().Format("2006-01-02 15:04:05"), info.Size())
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

	loadChatHistory(selected)
}

// printMemoryEntries 打印记忆条目列表
func printMemoryEntries(entries []memory.MemoryEntry) {
	typeEmoji := map[string]string{
		"preference": "💡", "fact": "📌", "lesson": "⚠️",
		"decision": "✅", "insight": "🔍",
	}

	for i, e := range entries {
		emoji := typeEmoji[string(e.Type)]
		if emoji == "" {
			emoji = "📝"
		}
		pterm.Printf("  %d. %s [%s] %s\n", i+1, emoji, e.Type, e.Summary)
		pterm.Printf("     %s\n", e.Detail)
		pterm.Printf("     标签: %s | 命中: %d 次 | 创建: %s\n",
			strings.Join(e.Tags, ", "), e.HitCount, e.CreatedAt.Format("2006-01-02 15:04"))
		pterm.Println()
	}
}

// handleClearMemory 清空所有长期记忆
func handleClearMemory() {
	if g.MemoryManager == nil {
		pterm.Error.Println("长期记忆未启用，请设置 FEIKONG_MEMORY_ENABLED=true")
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

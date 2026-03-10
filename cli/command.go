package cli

import (
	"context"
	"fkteams/agents"
	"fkteams/agents/leader"
	"fkteams/fkevent"
	"fkteams/g"
	"fkteams/tools/scheduler"
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
		pterm.Info.Println("谢谢使用，再见！")
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
		pterm.Println("  list_chat_history                        列出所有可用的聊天历史会话")
		pterm.Println("  load_chat_history <session_id>            加载指定的聊天历史会话")
		pterm.Println("  save_chat_history                        保存聊天历史到当前会话文件")
		pterm.Println("  clear_chat_history              清空当前聊天历史")
		pterm.Println("  save_chat_history_to_markdown   导出聊天历史为 Markdown 文件")
		pterm.Println("  save_chat_history_to_html       导出聊天历史为 HTML 文件")
		pterm.Println()
		pterm.Println("任务管理:")
		pterm.Println("  clear_todo                      清空所有待办事项")
		pterm.Println("  list_schedule                   列出所有定时任务")
		pterm.Println("  cancel_schedule <id>            取消指定的定时任务")
		pterm.Println()
		pterm.Println("模式切换:")
		pterm.Println("  switch_work_mode               切换当前工作模式")
		pterm.Println()
		pterm.Println("长期记忆管理:")
		pterm.Println("  list_memory                    列出所有长期记忆条目")
		pterm.Println("  delete_memory <摘要>           删除指定摘要的记忆条目")
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
		historyFile := CLIHistoryDir + "fkteams_chat_history_" + activeSessionID
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

	case "clear_todo":
		err := leader.ClearTodoTool()
		if err != nil {
			pterm.Error.Printfln("清空待办事项失败: %v", err)
		} else {
			pterm.Success.Println("成功清空待办事项")
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

	case "clear_memory":
		handleClearMemory()
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
		// 支持 load_chat_history <session_id> 格式
		if strings.HasPrefix(input, "load_chat_history ") {
			sessionID := strings.TrimSpace(strings.TrimPrefix(input, "load_chat_history "))
			if sessionID != "" {
				loadChatHistory(sessionID)
				return ResultHandled
			}
		}

		// 支持 cancel_schedule <task_id> 格式
		if strings.HasPrefix(input, "cancel_schedule ") {
			taskID := strings.TrimSpace(strings.TrimPrefix(input, "cancel_schedule "))
			if taskID != "" {
				s := scheduler.Global()
				if s == nil {
					pterm.Error.Println("定时任务调度器未初始化")
					return ResultHandled
				}
				ctx := context.Background()
				resp, _ := s.ScheduleCancel(ctx, &scheduler.ScheduleCancelRequest{TaskID: taskID})
				if resp.ErrorMessage != "" {
					pterm.Error.Println(resp.ErrorMessage)
				} else {
					pterm.Success.Println(resp.Message)
				}
				return ResultHandled
			}
		}

		// 支持 delete_memory <summary> 格式
		if strings.HasPrefix(input, "delete_memory ") {
			summary := strings.TrimSpace(strings.TrimPrefix(input, "delete_memory "))
			if summary != "" {
				handleDeleteMemory(summary)
				return ResultHandled
			}
		}

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
		if !strings.HasPrefix(name, "fkteams_chat_history_") {
			continue
		}
		sessionID := strings.TrimPrefix(name, "fkteams_chat_history_")
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
	historyFile := CLIHistoryDir + "fkteams_chat_history_" + sessionID
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
	if g.MemManager == nil {
		pterm.Error.Println("长期记忆未启用，请设置 FEIKONG_MEMORY_ENABLED=true")
		return
	}

	entries := g.MemManager.List()
	if len(entries) == 0 {
		pterm.Info.Println("暂无长期记忆条目")
		return
	}

	pterm.Println()
	pterm.Println("=== 长期记忆列表 ===")
	pterm.Println()

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

	pterm.Printf("共 %d 条记忆\n", len(entries))
	pterm.Printf("使用 delete_memory <摘要> 删除指定条目，或 clear_memory 清空全部\n")
	pterm.Println()
}

// handleDeleteMemory 删除指定摘要的记忆条目
func handleDeleteMemory(summary string) {
	if g.MemManager == nil {
		pterm.Error.Println("长期记忆未启用，请设置 FEIKONG_MEMORY_ENABLED=true")
		return
	}

	deleted := g.MemManager.Delete(summary)
	if deleted > 0 {
		pterm.Success.Printfln("成功删除 %d 条记忆: %s", deleted, summary)
	} else {
		pterm.Warning.Printfln("未找到匹配的记忆条目: %s", summary)
		pterm.Info.Println("使用 list_memory 查看所有记忆条目")
	}
}

// handleClearMemory 清空所有长期记忆
func handleClearMemory() {
	if g.MemManager == nil {
		pterm.Error.Println("长期记忆未启用，请设置 FEIKONG_MEMORY_ENABLED=true")
		return
	}

	count := g.MemManager.Count()
	if count == 0 {
		pterm.Info.Println("当前没有记忆条目")
		return
	}

	g.MemManager.Clear()
	pterm.Success.Printfln("成功清空 %d 条长期记忆", count)
}

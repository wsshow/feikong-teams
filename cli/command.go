package cli

import (
	"fkteams/agents"
	"fkteams/agents/leader"
	"fkteams/fkevent"
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
		pterm.Println()
		pterm.Println("模式切换:")
		pterm.Println("  switch_work_mode               切换当前工作模式")
		pterm.Println()
		pterm.Println("其他操作:")
		pterm.Println("  直接输入问题                     与智能体团队对话")
		return ResultHandled

	case "list_chat_history":
		listChatHistoryFiles()
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

	default:
		// 支持 load_chat_history <session_id> 格式
		if strings.HasPrefix(input, "load_chat_history ") {
			sessionID := strings.TrimSpace(strings.TrimPrefix(input, "load_chat_history "))
			if sessionID != "" {
				loadChatHistory(sessionID)
				return ResultHandled
			}
		}
		return ResultNotFound
	}
}

// listChatHistoryFiles 列出所有可用的聊天历史文件
func listChatHistoryFiles() {
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
		pterm.Printf("共 %d 个会话，使用 load_chat_history <session_id> 加载\n", count)
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

package cli

import (
	"fkteams/agents"
	"fkteams/agents/leader"
	"fkteams/fkevent"

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
		pterm.Println("聊天历史管理:")
		pterm.Println("  load_chat_history               从默认文件加载聊天历史")
		pterm.Println("  save_chat_history               保存聊天历史到默认文件")
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

	case "load_chat_history":
		err := fkevent.GlobalHistoryRecorder.LoadFromDefaultFile()
		if err != nil {
			pterm.Error.Printfln("加载聊天历史失败: %v", err)
		} else {
			pterm.Success.Println("成功加载聊天历史")
		}
		return ResultHandled

	case "save_chat_history":
		err := fkevent.GlobalHistoryRecorder.SaveToDefaultFile()
		if err != nil {
			pterm.Error.Printfln("保存聊天历史失败: %v", err)
		} else {
			pterm.Success.Println("成功保存聊天历史")
		}
		return ResultHandled

	case "clear_chat_history":
		fkevent.GlobalHistoryRecorder.Clear()
		pterm.Success.Println("成功清空当前聊天历史")
		return ResultHandled

	case "save_chat_history_to_markdown":
		filePath, err := fkevent.GlobalHistoryRecorder.SaveToMarkdownWithTimestamp()
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
		return ResultNotFound
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

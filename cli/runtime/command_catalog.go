package runtime

// CommandInfo 命令信息。
type CommandInfo struct {
	Name     string
	Desc     string
	Usage    string
	Category string
}

// allCommands 是 TUI runtime 命令补全列表。
var allCommands = []CommandInfo{
	{Name: "help", Desc: "显示帮助信息", Category: "基础"},
	{Name: "quit", Desc: "退出程序", Category: "基础"},
	{Name: "list_agents", Desc: "列出所有可用的智能体", Category: "基础"},
	{Name: "switch_work_mode", Desc: "切换工作模式", Usage: "[team|deep|group|custom]", Category: "基础"},

	{Name: "list_chat_history", Desc: "列出所有聊天历史会话", Category: "会话"},
	{Name: "load_chat_history", Desc: "选择并加载聊天历史会话", Usage: "[SESSION_ID]", Category: "会话"},
	{Name: "save_chat_history", Desc: "保存聊天历史到当前会话文件", Category: "会话"},
	{Name: "clear_chat_history", Desc: "清空当前聊天历史", Category: "会话"},
	{Name: "save_chat_history_to_html", Desc: "导出聊天历史为 HTML 文件", Category: "会话"},
	{Name: "save_chat_history_to_markdown", Desc: "导出聊天历史为 Markdown 文件", Category: "会话"},

	{Name: "list_schedule", Desc: "列出所有定时任务", Category: "定时任务"},
	{Name: "cancel_schedule", Desc: "选择并取消定时任务", Usage: "[TASK_ID]", Category: "定时任务"},
	{Name: "delete_schedule", Desc: "选择并删除定时任务", Usage: "[TASK_ID]", Category: "定时任务"},

	{Name: "list_memory", Desc: "列出所有长期记忆条目", Category: "长期记忆"},
	{Name: "delete_memory", Desc: "选择并删除记忆条目", Usage: "[SUMMARY]", Category: "长期记忆"},
	{Name: "clear_memory", Desc: "清空所有长期记忆", Category: "长期记忆"},
}

func commandInfoByName(name string) (CommandInfo, bool) {
	for _, command := range allCommands {
		if command.Name == name {
			return command, true
		}
	}
	return CommandInfo{}, false
}

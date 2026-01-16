package fkevent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ToolCallRecord 工具调用记录
type ToolCallRecord struct {
	Name      string `json:"name"`      // 工具名称
	Arguments string `json:"arguments"` // 工具参数
	Result    string `json:"result"`    // 工具执行结果
}

// ActionRecord action 事件记录
type ActionRecord struct {
	ActionType string `json:"action_type"` // action 类型（transfer、exit、interrupted 等）
	Content    string `json:"content"`     // action 内容描述
}

// AgentMessage 代理的一次完整发言
type AgentMessage struct {
	AgentName string           `json:"agent_name"`           // 代理名称
	Content   string           `json:"content"`              // 完整内容
	RunPath   string           `json:"run_path"`             // 运行路径
	StartTime time.Time        `json:"start_time"`           // 开始时间
	EndTime   time.Time        `json:"end_time"`             // 结束时间（当切换到下一个 agent 时）
	ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"` // 工具调用记录
	Actions   []ActionRecord   `json:"actions,omitempty"`    // action 事件记录
}

// HistoryRecorder 记录事件历史
type HistoryRecorder struct {
	mu               sync.RWMutex
	messages         []AgentMessage    // 按时间顺序保存所有发言
	currentBuilder   *strings.Builder  // 当前正在构建的内容
	currentAgent     string            // 当前正在发言的 agent
	currentRunPath   string            // 当前运行路径
	currentStartTime time.Time         // 当前发言开始时间
	currentToolCalls []ToolCallRecord  // 当前消息的工具调用记录
	pendingToolCalls map[string]string // 待完成的工具调用（工具名 -> 参数）
	currentActions   []ActionRecord    // 当前消息的 action 事件记录
}

// NewHistoryRecorder 创建新的历史记录器
func NewHistoryRecorder() *HistoryRecorder {
	return &HistoryRecorder{
		messages:         make([]AgentMessage, 0),
		currentBuilder:   &strings.Builder{},
		currentToolCalls: make([]ToolCallRecord, 0),
		pendingToolCalls: make(map[string]string),
		currentActions:   make([]ActionRecord, 0),
	}
}

// RecordEvent 记录事件
func (h *HistoryRecorder) RecordEvent(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 处理文本内容事件
	if event.Type == "stream_chunk" {
		// 如果是新的 agent 开始发言，保存上一个 agent 的内容
		if event.AgentName != h.currentAgent && h.currentAgent != "" {
			h.finalizeCurrentMessage()
		}

		// 如果是新的 agent 或第一次记录
		if event.AgentName != h.currentAgent {
			h.currentAgent = event.AgentName
			h.currentRunPath = event.RunPath
			h.currentStartTime = time.Now()
			h.currentBuilder.Reset()
			h.currentToolCalls = make([]ToolCallRecord, 0)
			h.pendingToolCalls = make(map[string]string)
			h.currentActions = make([]ActionRecord, 0)
		}

		// 累积当前内容
		h.currentBuilder.WriteString(event.Content)
		return
	}

	// 处理工具调用准备事件
	if event.Type == "tool_calls_preparing" || event.Type == "tool_calls" {
		if len(event.ToolCalls) > 0 {
			for _, tc := range event.ToolCalls {
				// 记录待处理的工具调用
				h.pendingToolCalls[tc.Function.Name] = tc.Function.Arguments
			}
		}
		return
	}

	// 处理工具结果事件
	if event.Type == "tool_result" || event.Type == "tool_result_chunk" {
		// 尝试匹配待处理的工具调用
		for toolName, args := range h.pendingToolCalls {
			// 将工具结果添加到当前消息的工具调用记录中
			h.currentToolCalls = append(h.currentToolCalls, ToolCallRecord{
				Name:      toolName,
				Arguments: args,
				Result:    event.Content,
			})
			// 清除已处理的工具调用
			delete(h.pendingToolCalls, toolName)
			break // 一次只处理一个结果
		}
		return
	}

	// 处理 action 事件（如 transfer、exit 等）
	if event.Type == "action" {
		// 将 action 事件添加到当前消息的 action 记录中
		h.currentActions = append(h.currentActions, ActionRecord{
			ActionType: event.ActionType,
			Content:    event.Content,
		})
		return
	}
}

// RecordUserInput 记录用户输入
func (h *HistoryRecorder) RecordUserInput(input string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 完成当前 agent 的消息（如果有）
	if h.currentAgent != "" {
		h.finalizeCurrentMessage()
	}

	// 记录用户输入为特殊的消息
	h.messages = append(h.messages, AgentMessage{
		AgentName: "用户",
		Content:   input,
		RunPath:   "",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	})

	// 重置当前状态
	h.currentAgent = ""
	h.currentBuilder.Reset()
}

// finalizeCurrentMessage 完成当前消息的记录
func (h *HistoryRecorder) finalizeCurrentMessage() {
	if h.currentAgent == "" {
		return
	}

	content := h.currentBuilder.String()
	if content != "" {
		msg := AgentMessage{
			AgentName: h.currentAgent,
			Content:   content,
			RunPath:   h.currentRunPath,
			StartTime: h.currentStartTime,
			EndTime:   time.Now(),
		}
		// 如果有工具调用记录，添加到消息中
		if len(h.currentToolCalls) > 0 {
			msg.ToolCalls = h.currentToolCalls
		}
		// 如果有 action 事件记录，添加到消息中
		if len(h.currentActions) > 0 {
			msg.Actions = h.currentActions
		}
		h.messages = append(h.messages, msg)
	}
}

// FinalizeCurrent 手动完成当前消息记录（在对话结束时调用）
func (h *HistoryRecorder) FinalizeCurrent() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finalizeCurrentMessage()
	h.currentAgent = ""
	h.currentBuilder.Reset()
	h.currentToolCalls = make([]ToolCallRecord, 0)
	h.pendingToolCalls = make(map[string]string)
	h.currentActions = make([]ActionRecord, 0)
}

// GetMessages 获取所有消息（按时间顺序）
func (h *HistoryRecorder) GetMessages() []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 复制一份返回
	result := make([]AgentMessage, len(h.messages))
	copy(result, h.messages)
	return result
}

// GetAgentMessages 获取指定 agent 的所有消息（按时间顺序）
func (h *HistoryRecorder) GetAgentMessages(agentName string) []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]AgentMessage, 0)
	for _, msg := range h.messages {
		if msg.AgentName == agentName {
			result = append(result, msg)
		}
	}
	return result
}

// GetCurrentMessage 获取当前正在构建的消息
func (h *HistoryRecorder) GetCurrentMessage() (string, string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.currentAgent, h.currentBuilder.String()
}

// GetFullHistory 获取完整的对话历史（格式化字符串）
func (h *HistoryRecorder) GetFullHistory() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result strings.Builder
	for i, msg := range h.messages {
		if i > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("=== ")
		result.WriteString(msg.AgentName)
		result.WriteString(" ===\n")
		result.WriteString(msg.Content)
	}

	// 包含当前正在构建的内容
	if h.currentAgent != "" && h.currentBuilder.Len() > 0 {
		if len(h.messages) > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("=== ")
		result.WriteString(h.currentAgent)
		result.WriteString(" (当前) ===\n")
		result.WriteString(h.currentBuilder.String())
	}

	return result.String()
}

// GetConversationSummary 获取对话摘要
func (h *HistoryRecorder) GetConversationSummary() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result strings.Builder
	for i, msg := range h.messages {
		duration := msg.EndTime.Sub(msg.StartTime)
		contentLen := len([]rune(msg.Content))
		result.WriteString(fmt.Sprintf("%d. [%s] %s - %d字 (%v)\n",
			i+1, msg.StartTime.Format("15:04:05"), msg.AgentName, contentLen, duration.Round(time.Millisecond)))
	}
	return result.String()
}

// Clear 清空所有历史记录
func (h *HistoryRecorder) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = make([]AgentMessage, 0)
	h.currentBuilder.Reset()
	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentToolCalls = make([]ToolCallRecord, 0)
	h.pendingToolCalls = make(map[string]string)
	h.currentActions = make([]ActionRecord, 0)
}

// GetAgentNames 获取所有参与对话的 agent 名称（去重）
func (h *HistoryRecorder) GetAgentNames() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	nameMap := make(map[string]bool)
	for _, msg := range h.messages {
		nameMap[msg.AgentName] = true
	}
	if h.currentAgent != "" {
		nameMap[h.currentAgent] = true
	}

	names := make([]string, 0, len(nameMap))
	for name := range nameMap {
		names = append(names, name)
	}
	return names
}

// GetMessageCount 获取消息总数
func (h *HistoryRecorder) GetMessageCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.messages)
}

// SaveToFile 保存历史对话到文件
func (h *HistoryRecorder) SaveToFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 确保当前消息已完成
	if h.currentAgent != "" && h.currentBuilder.Len() > 0 {
		// 临时创建一个包含当前消息的副本
		tempMessages := make([]AgentMessage, len(h.messages))
		copy(tempMessages, h.messages)
		msg := AgentMessage{
			AgentName: h.currentAgent,
			Content:   h.currentBuilder.String(),
			RunPath:   h.currentRunPath,
			StartTime: h.currentStartTime,
			EndTime:   time.Now(),
		}
		if len(h.currentToolCalls) > 0 {
			msg.ToolCalls = h.currentToolCalls
		}
		if len(h.currentActions) > 0 {
			msg.Actions = h.currentActions
		}
		tempMessages = append(tempMessages, msg)
		return saveMessagesToFile(tempMessages, filePath)
	}

	return saveMessagesToFile(h.messages, filePath)
}

// saveMessagesToFile 内部函数：保存消息到文件
func saveMessagesToFile(messages []AgentMessage, filePath string) error {
	// 创建目录（如果不存在）
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	// 序列化为 JSON
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// LoadFromFile 从文件加载历史对话
func (h *HistoryRecorder) LoadFromFile(filePath string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 读取文件
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 反序列化
	var messages []AgentMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return fmt.Errorf("反序列化失败: %w", err)
	}

	// 替换当前消息
	h.messages = messages
	h.currentAgent = ""
	h.currentBuilder.Reset()
	h.currentRunPath = ""
	h.currentToolCalls = make([]ToolCallRecord, 0)
	h.pendingToolCalls = make(map[string]string)
	h.currentActions = make([]ActionRecord, 0)

	return nil
}

// SaveToDefaultFile 保存到默认文件路径
func (h *HistoryRecorder) SaveToDefaultFile() error {
	return h.SaveToFile("./history/chat_history/fkteams_chat_history")
}

// LoadFromDefaultFile 从默认文件路径加载
func (h *HistoryRecorder) LoadFromDefaultFile() error {
	return h.LoadFromFile("./history/chat_history/fkteams_chat_history")
}

// SaveToTimestampedFile 保存到带时间戳的文件
func (h *HistoryRecorder) SaveToTimestampedFile() (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filePath := fmt.Sprintf("./history/chat_history/fkteams_chat_history_%s.json", timestamp)
	err := h.SaveToFile(filePath)
	return filePath, err
}

// ListHistoryFiles 列出历史文件目录中的所有文件
func ListHistoryFiles() ([]string, error) {
	dir := "./history/chat_history/"

	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

// SaveToMarkdownFile 保存历史对话为 Markdown 格式文件
func (h *HistoryRecorder) SaveToMarkdownFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 收集所有消息（包括当前正在构建的）
	messages := make([]AgentMessage, len(h.messages))
	copy(messages, h.messages)

	if h.currentAgent != "" && h.currentBuilder.Len() > 0 {
		msg := AgentMessage{
			AgentName: h.currentAgent,
			Content:   h.currentBuilder.String(),
			RunPath:   h.currentRunPath,
			StartTime: h.currentStartTime,
			EndTime:   time.Now(),
		}
		if len(h.currentToolCalls) > 0 {
			msg.ToolCalls = h.currentToolCalls
		}
		if len(h.currentActions) > 0 {
			msg.Actions = h.currentActions
		}
		messages = append(messages, msg)
	}

	return saveMessagesToMarkdown(messages, filePath)
}

// saveMessagesToMarkdown 内部函数：保存消息为 Markdown 格式
func saveMessagesToMarkdown(messages []AgentMessage, filePath string) error {
	// 创建目录（如果不存在）
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}

	var md strings.Builder

	// 文件头部
	md.WriteString("# 对话历史\n\n")
	md.WriteString(fmt.Sprintf("**生成时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	md.WriteString(fmt.Sprintf("**对话轮次**: %d\n\n", len(messages)))

	// 统计参与的代理
	agentMap := make(map[string]int)
	for _, msg := range messages {
		agentMap[msg.AgentName]++
	}
	md.WriteString("**参与代理**: ")
	first := true
	for agent, count := range agentMap {
		if !first {
			md.WriteString(", ")
		}
		md.WriteString(fmt.Sprintf("%s (%d次)", agent, count))
		first = false
	}
	md.WriteString("\n\n---\n\n")

	// 对话内容
	for i, msg := range messages {
		// 消息序号和代理名称
		md.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, msg.AgentName))

		// 时间信息
		duration := msg.EndTime.Sub(msg.StartTime)
		md.WriteString(fmt.Sprintf("**时间**: %s - %s (%v)\n\n",
			msg.StartTime.Format("15:04:05"),
			msg.EndTime.Format("15:04:05"),
			duration.Round(time.Millisecond)))

		// 运行路径（如果有）
		if msg.RunPath != "" {
			md.WriteString(fmt.Sprintf("**路径**: `%s`\n\n", msg.RunPath))
		}

		// 工具调用（如果有）
		if len(msg.ToolCalls) > 0 {
			md.WriteString("**工具调用**:\n\n")
			for i, tc := range msg.ToolCalls {
				md.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, tc.Name))
				if tc.Arguments != "" {
					md.WriteString(fmt.Sprintf("   - 参数: `%s`\n", tc.Arguments))
				}
				if tc.Result != "" {
					md.WriteString(fmt.Sprintf("   - 结果: %s\n", tc.Result))
				}
			}
			md.WriteString("\n")
		}

		// Action 事件（如果有）
		if len(msg.Actions) > 0 {
			md.WriteString("**Action 事件**:\n\n")
			for i, action := range msg.Actions {
				md.WriteString(fmt.Sprintf("%d. **[%s]** %s\n", i+1, action.ActionType, action.Content))
			}
			md.WriteString("\n")
		}

		// 内容
		md.WriteString("**内容**:\n\n")
		md.WriteString(msg.Content)
		md.WriteString("\n\n")

		// 分隔线（除了最后一条消息）
		if i < len(messages)-1 {
			md.WriteString("---\n\n")
		}
	}

	// 写入文件
	if err := os.WriteFile(filePath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// SaveToMarkdownWithTimestamp 保存为带时间戳的 Markdown 文件
func (h *HistoryRecorder) SaveToMarkdownWithTimestamp() (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filePath := fmt.Sprintf("./history/chat_history/fkteams_chat_history_%s.md", timestamp)
	err := h.SaveToMarkdownFile(filePath)
	return filePath, err
}

// SaveToDefaultMarkdownFile 保存到默认 Markdown 文件路径
func (h *HistoryRecorder) SaveToDefaultMarkdownFile() error {
	return h.SaveToMarkdownFile("./history/chat_history/fkteams_chat_history.md")
}

// 全局历史记录器实例
var GlobalHistoryRecorder = NewHistoryRecorder()

// RecordEventWithHistory 记录事件到历史并打印
func RecordEventWithHistory(event Event) {
	// 记录到历史
	GlobalHistoryRecorder.RecordEvent(event)

	// 打印事件
	PrintEvent(event)
}

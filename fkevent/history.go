package fkevent

import (
	"encoding/json"
	"fkteams/common"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ToolCallRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Target      string `json:"target,omitempty"`
	Arguments   string `json:"arguments"`
	Result      string `json:"result"`
}

type ActionRecord struct {
	ActionType ActionType `json:"action_type"`
	Content    string     `json:"content"`
	Detail     string     `json:"detail,omitempty"`
}

// HistoryData 持久化数据结构
type HistoryData struct {
	Messages []AgentMessage `json:"messages"`
}

// MsgEventType 历史消息事件类型
type MsgEventType string

const (
	MsgTypeText      MsgEventType = "text"
	MsgTypeReasoning MsgEventType = "reasoning"
	MsgTypeToolCall  MsgEventType = "tool_call"
	MsgTypeAction    MsgEventType = "action"
	MsgTypeError     MsgEventType = "error"
)

// MessageEvent 单个消息事件
type MessageEvent struct {
	Type     MsgEventType    `json:"type"`
	Content  string          `json:"content,omitempty"`
	ToolCall *ToolCallRecord `json:"tool_call,omitempty"`
	Action   *ActionRecord   `json:"action,omitempty"`
}

// AgentMessage 代理的一次完整发言
type AgentMessage struct {
	AgentName      string         `json:"agent_name"`
	RunPath        string         `json:"run_path"`
	MemberCallID   string         `json:"member_call_id,omitempty"`
	MemberToolName string         `json:"member_tool_name,omitempty"`
	MemberName     string         `json:"member_name,omitempty"`
	IsMemberEvent  bool           `json:"is_member_event,omitempty"`
	StartTime      time.Time      `json:"start_time"`
	EndTime        time.Time      `json:"end_time"`
	Events         []MessageEvent `json:"events"`
}

// GetTextContent 获取消息中的所有文本内容
func (m *AgentMessage) GetTextContent() string {
	var builder strings.Builder
	for _, event := range m.Events {
		if event.Type == MsgTypeText {
			builder.WriteString(event.Content)
		}
	}
	return builder.String()
}

// NoisyToolPrefixes 定义高输出量噪声工具的名称前缀列表。
// 这类工具（如网页抓取、文档读取）会产生大量输出，在历史上下文中属于冗余内容。
var NoisyToolPrefixes = []string{"fetch", "doc"}

// maxToolArgLen 工具参数摘要最大长度（rune）
const maxToolArgLen = 200

// maxToolResultLen 工具结果摘要最大长度（rune）
const maxToolResultLen = 2000

// truncateRunes 按 rune 截断字符串
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// GetReasoningContent 获取消息中的推理/思考内容
func (m *AgentMessage) GetReasoningContent() string {
	var builder strings.Builder
	for _, event := range m.Events {
		if event.Type == MsgTypeReasoning {
			builder.WriteString(event.Content)
		}
	}
	return builder.String()
}

// 错误内容最大长度（rune），超出时保留头尾并截断中间部分
const maxErrorContentLen = 1200

// pendingToolCall 待匹配的工具调用
type pendingToolCall struct {
	ID          string
	Index       *int
	Name        string
	DisplayName string
	Kind        string
	Target      string
	Arguments   string
}

// HistoryRecorder 事件历史记录器
type HistoryRecorder struct {
	mu                sync.RWMutex
	messages          []AgentMessage
	currentAgent      string
	currentRunPath    string
	currentMemberID   string
	currentMemberTool string
	currentMemberName string
	currentStartTime  time.Time
	currentEvents     []MessageEvent
	pendingToolCalls  []pendingToolCall // 按 ID 匹配工具调用与结果
	toolResultChunks  map[string]string // 按 ID 累积流式工具结果
	summary           string            // 上下文压缩摘要
	summarizedCount   int               // 已被摘要覆盖的消息数量
}

func toolCallRecordFromPending(tc pendingToolCall, result string) ToolCallRecord {
	record := ToolCallRecord{
		ID:          tc.ID,
		Name:        tc.Name,
		DisplayName: tc.DisplayName,
		Kind:        tc.Kind,
		Target:      tc.Target,
		Arguments:   tc.Arguments,
		Result:      result,
	}
	if record.DisplayName == "" || record.Kind == "" {
		display := FormatToolDisplay(tc.Name)
		if record.DisplayName == "" {
			record.DisplayName = display.DisplayName
		}
		if record.Kind == "" {
			record.Kind = display.Kind
		}
		if record.Target == "" {
			record.Target = display.Target
		}
	}
	return record
}

func ptrToolCallRecord(record ToolCallRecord) *ToolCallRecord {
	return &record
}

func pendingToolCallFromEvent(id string, index *int, name, arguments string) pendingToolCall {
	display := FormatToolDisplay(name)
	return pendingToolCall{
		ID:          id,
		Index:       index,
		Name:        name,
		DisplayName: display.DisplayName,
		Kind:        display.Kind,
		Target:      display.Target,
		Arguments:   arguments,
	}
}

func NewHistoryRecorder() *HistoryRecorder {
	return &HistoryRecorder{
		messages:         make([]AgentMessage, 0),
		currentEvents:    make([]MessageEvent, 0),
		toolResultChunks: make(map[string]string),
	}
}

// truncateErrorContent 截断过长的错误内容，保留头尾部分
func truncateErrorContent(s string) string {
	runes := []rune(s)
	if len(runes) <= maxErrorContentLen {
		return s
	}
	head := maxErrorContentLen * 2 / 3
	tail := maxErrorContentLen - head
	return string(runes[:head]) + "\n...(truncated)...\n" + string(runes[len(runes)-tail:])
}

func toolResultKey(event Event) string {
	if event.ToolCallID != "" {
		return event.ToolCallID
	}
	if event.ToolCallIndex != nil {
		return fmt.Sprintf("idx:%d", *event.ToolCallIndex)
	}
	return ""
}

// RecordEvent 记录事件
func (h *HistoryRecorder) RecordEvent(event Event) {
	event = NormalizeEvent(event)

	h.mu.Lock()
	defer h.mu.Unlock()

	switch event.Type {
	case EventReasoningChunk:
		h.ensureAgentContext(event)
		// 合并连续推理事件
		if n := len(h.currentEvents); n > 0 && h.currentEvents[n-1].Type == MsgTypeReasoning {
			h.currentEvents[n-1].Content += event.Content
		} else {
			h.currentEvents = append(h.currentEvents, MessageEvent{
				Type:    MsgTypeReasoning,
				Content: event.Content,
			})
		}

	case EventStreamChunk:
		h.ensureAgentContext(event)
		// 合并连续文本事件
		if n := len(h.currentEvents); n > 0 && h.currentEvents[n-1].Type == MsgTypeText {
			h.currentEvents[n-1].Content += event.Content
		} else {
			h.currentEvents = append(h.currentEvents, MessageEvent{
				Type:    MsgTypeText,
				Content: event.Content,
			})
		}

	case EventToolCallsPreparing:
		h.ensureAgentContext(event)
		for _, tc := range event.ToolCalls {
			if isInternalToolName(tc.Function.Name) {
				continue
			}
			if tc.Function.Name != "" {
				h.pendingToolCalls = append(h.pendingToolCalls,
					pendingToolCallFromEvent(tc.ID, tc.Index, tc.Function.Name, ""))
			}
		}

	case EventToolCalls:
		h.ensureAgentContext(event)
		for _, tc := range event.ToolCalls {
			if isInternalToolName(tc.Function.Name) {
				continue
			}
			updated := false
			for i := range h.pendingToolCalls {
				sameID := h.pendingToolCalls[i].ID != "" && h.pendingToolCalls[i].ID == tc.ID
				sameIndex := h.pendingToolCalls[i].Index != nil && tc.Index != nil && *h.pendingToolCalls[i].Index == *tc.Index
				if (sameID || sameIndex) && h.pendingToolCalls[i].Arguments == "" {
					if tc.ID != "" {
						h.pendingToolCalls[i].ID = tc.ID
					}
					h.pendingToolCalls[i].Arguments = tc.Function.Arguments
					updated = true
					break
				}
			}
			if !updated {
				h.pendingToolCalls = append(h.pendingToolCalls,
					pendingToolCallFromEvent(tc.ID, tc.Index, tc.Function.Name, tc.Function.Arguments))
			}
		}

	case EventToolResultChunk:
		if isInternalContinueContent(event.Content) {
			return
		}
		h.ensureAgentContext(event)
		if key := toolResultKey(event); key != "" {
			h.toolResultChunks[key] += event.Content
		}

	case EventToolResult:
		if isInternalContinueContent(event.Content) {
			return
		}
		h.ensureAgentContext(event)
		content := event.Content
		resultKey := toolResultKey(event)
		if resultKey != "" && h.toolResultChunks[resultKey] != "" {
			chunked := h.toolResultChunks[resultKey]
			if content == "" || strings.Contains(chunked, content) {
				content = chunked
			} else {
				content = chunked + content
			}
			delete(h.toolResultChunks, resultKey)
		}
		idx := -1
		for i := range h.pendingToolCalls {
			sameID := h.pendingToolCalls[i].ID != "" && h.pendingToolCalls[i].ID == event.ToolCallID
			sameIndex := h.pendingToolCalls[i].Index != nil && event.ToolCallIndex != nil && *h.pendingToolCalls[i].Index == *event.ToolCallIndex
			if sameID || sameIndex {
				idx = i
				break
			}
		}
		if idx >= 0 {
			tc := h.pendingToolCalls[idx]
			h.pendingToolCalls = append(h.pendingToolCalls[:idx], h.pendingToolCalls[idx+1:]...)
			if isInternalToolName(tc.Name) {
				return
			}
			h.currentEvents = append(h.currentEvents, MessageEvent{
				Type:     MsgTypeToolCall,
				ToolCall: ptrToolCallRecord(toolCallRecordFromPending(tc, content)),
			})
		}

	case EventMessage:
		h.ensureAgentContext(event)
		if event.ReasoningContent != "" {
			h.currentEvents = append(h.currentEvents, MessageEvent{
				Type:    MsgTypeReasoning,
				Content: event.ReasoningContent,
			})
		}
		if event.Content != "" {
			h.currentEvents = append(h.currentEvents, MessageEvent{
				Type:    MsgTypeText,
				Content: event.Content,
			})
		}
		for _, tc := range event.ToolCalls {
			if isInternalToolName(tc.Function.Name) {
				continue
			}
			if tc.Function.Name != "" {
				h.pendingToolCalls = append(h.pendingToolCalls,
					pendingToolCallFromEvent(tc.ID, tc.Index, tc.Function.Name, tc.Function.Arguments))
			}
		}

	case EventAction:
		h.currentEvents = append(h.currentEvents, MessageEvent{
			Type: MsgTypeAction,
			Action: &ActionRecord{
				ActionType: event.ActionType,
				Content:    event.Content,
				Detail:     event.Detail,
			},
		})

	case EventError:
		h.ensureAgentContext(event)
		h.currentEvents = append(h.currentEvents, MessageEvent{
			Type:    MsgTypeError,
			Content: truncateErrorContent(event.Error),
		})

	case EventDispatchProgress:
		// 子任务进度事件不单独记录，最终结果已包含在 tool_call 的 result 中
	}
}

// ensureAgentContext 确保当前 agent 上下文已初始化，处理 agent/member 切换
func (h *HistoryRecorder) ensureAgentContext(event Event) {
	sameAgent := event.AgentName == h.currentAgent
	sameMember := event.MemberCallID == h.currentMemberID
	if h.currentAgent != "" && (!sameAgent || !sameMember) {
		h.finalizeCurrentMessage()
	}
	if !sameAgent || !sameMember {
		h.currentAgent = event.AgentName
		h.currentRunPath = event.RunPath
		h.currentMemberID = event.MemberCallID
		h.currentMemberTool = event.MemberToolName
		h.currentMemberName = event.MemberName
		h.currentStartTime = time.Now()
		h.currentEvents = make([]MessageEvent, 0)
		h.pendingToolCalls = nil
	}
}

func (h *HistoryRecorder) flushChunkedToolResults() {
	if len(h.toolResultChunks) == 0 {
		return
	}
	for resultKey, content := range h.toolResultChunks {
		if content == "" {
			delete(h.toolResultChunks, resultKey)
			continue
		}
		idx := -1
		for i := range h.pendingToolCalls {
			sameID := h.pendingToolCalls[i].ID != "" && h.pendingToolCalls[i].ID == resultKey
			sameIndex := h.pendingToolCalls[i].Index != nil && resultKey == fmt.Sprintf("idx:%d", *h.pendingToolCalls[i].Index)
			if sameID || sameIndex {
				idx = i
				break
			}
		}
		if idx < 0 {
			delete(h.toolResultChunks, resultKey)
			continue
		}
		tc := h.pendingToolCalls[idx]
		h.pendingToolCalls = append(h.pendingToolCalls[:idx], h.pendingToolCalls[idx+1:]...)
		if !isInternalToolName(tc.Name) {
			h.currentEvents = append(h.currentEvents, MessageEvent{
				Type:     MsgTypeToolCall,
				ToolCall: ptrToolCallRecord(toolCallRecordFromPending(tc, content)),
			})
		}
		delete(h.toolResultChunks, resultKey)
	}
}

func (h *HistoryRecorder) RecordUserInput(input string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.currentAgent != "" {
		h.finalizeCurrentMessage()
	}

	h.messages = append(h.messages, AgentMessage{
		AgentName: "用户",
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Events: []MessageEvent{
			{Type: MsgTypeText, Content: input},
		},
	})

	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
}

func (h *HistoryRecorder) finalizeCurrentMessage() {
	if h.currentAgent == "" {
		return
	}
	h.flushChunkedToolResults()
	if len(h.currentEvents) == 0 {
		return
	}

	h.messages = append(h.messages, AgentMessage{
		AgentName:      h.currentAgent,
		RunPath:        h.currentRunPath,
		MemberCallID:   h.currentMemberID,
		MemberToolName: h.currentMemberTool,
		MemberName:     h.currentMemberName,
		IsMemberEvent:  h.currentMemberID != "",
		StartTime:      h.currentStartTime,
		EndTime:        time.Now(),
		Events:         h.currentEvents,
	})
}

// FinalizeCurrent 完成当前消息记录，在对话结束时调用
func (h *HistoryRecorder) FinalizeCurrent() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finalizeCurrentMessage()
	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
	h.currentEvents = make([]MessageEvent, 0)
	h.pendingToolCalls = nil
	h.toolResultChunks = make(map[string]string)
}

func (h *HistoryRecorder) GetMessages() []AgentMessage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]AgentMessage, len(h.messages))
	copy(result, h.messages)
	return result
}

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

// GetCurrentMessage 返回当前构建中的 (agentName, textContent)
func (h *HistoryRecorder) GetCurrentMessage() (agentName, content string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var builder strings.Builder
	for _, event := range h.currentEvents {
		if event.Type == MsgTypeText {
			builder.WriteString(event.Content)
		}
	}
	return h.currentAgent, builder.String()
}

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
		for _, event := range msg.Events {
			if event.Type == MsgTypeText {
				result.WriteString(event.Content)
			}
		}
	}

	// 包含构建中的内容
	if h.currentAgent != "" && len(h.currentEvents) > 0 {
		if len(h.messages) > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("=== ")
		result.WriteString(h.currentAgent)
		result.WriteString(" (当前) ===\n")
		for _, event := range h.currentEvents {
			if event.Type == MsgTypeText {
				result.WriteString(event.Content)
			}
		}
	}

	return result.String()
}

func (h *HistoryRecorder) GetConversationSummary() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result strings.Builder
	for i, msg := range h.messages {
		duration := msg.EndTime.Sub(msg.StartTime)
		var contentLen int
		for _, event := range msg.Events {
			if event.Type == MsgTypeText {
				contentLen += len([]rune(event.Content))
			}
		}
		fmt.Fprintf(&result, "%d. [%s] %s - %d字 (%v)\n",
			i+1, msg.StartTime.Format("15:04:05"), msg.AgentName, contentLen, duration.Round(time.Millisecond))
	}
	return result.String()
}

func (h *HistoryRecorder) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.messages = make([]AgentMessage, 0)
	h.currentEvents = make([]MessageEvent, 0)
	h.currentAgent = ""
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
	h.pendingToolCalls = nil
	h.toolResultChunks = make(map[string]string)
	h.summary = ""
	h.summarizedCount = 0
}

// SetSummary 设置上下文压缩摘要
func (h *HistoryRecorder) SetSummary(text string, count int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.summary = text
	h.summarizedCount = count
}

// GetSummary 获取上下文压缩摘要和已覆盖的消息数量
func (h *HistoryRecorder) GetSummary() (string, int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.summary, h.summarizedCount
}

// reconstructSummaryFromEvents 从事件流中重建上下文压缩摘要状态（需在持锁状态下调用）
func (h *HistoryRecorder) reconstructSummaryFromEvents() {
	h.summary = ""
	h.summarizedCount = 0

	// 从后向前查找最后一个 context_compress 事件
	for i := len(h.messages) - 1; i >= 0; i-- {
		for _, evt := range h.messages[i].Events {
			if evt.Type == MsgTypeAction && evt.Action != nil &&
				evt.Action.ActionType == ActionContextCompress && evt.Action.Detail != "" {
				h.summary = evt.Action.Detail
				// 向前查找该事件所属执行轮次的用户输入
				for j := i - 1; j >= 0; j-- {
					if h.messages[j].AgentName == "用户" {
						h.summarizedCount = j
						return
					}
				}
				h.summarizedCount = 0
				return
			}
		}
	}
}
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

func (h *HistoryRecorder) GetMessageCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.messages)
}

func (h *HistoryRecorder) SaveToFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.currentAgent != "" && len(h.currentEvents) > 0 {
		tempMessages := make([]AgentMessage, len(h.messages))
		copy(tempMessages, h.messages)
		msg := AgentMessage{
			AgentName:      h.currentAgent,
			RunPath:        h.currentRunPath,
			MemberCallID:   h.currentMemberID,
			MemberToolName: h.currentMemberTool,
			MemberName:     h.currentMemberName,
			IsMemberEvent:  h.currentMemberID != "",
			StartTime:      h.currentStartTime,
			EndTime:        time.Now(),
			Events:         h.currentEvents,
		}
		tempMessages = append(tempMessages, msg)
		return saveMessagesToFile(tempMessages, filePath)
	}

	return saveMessagesToFile(h.messages, filePath)
}

func saveMessagesToFile(messages []AgentMessage, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	histData := HistoryData{
		Messages: messages,
	}
	data, err := json.MarshalIndent(histData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (h *HistoryRecorder) LoadFromFile(filePath string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var histData HistoryData
	if err := json.Unmarshal(data, &histData); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	h.messages = histData.Messages

	// 从事件流中重建上下文压缩摘要状态
	h.reconstructSummaryFromEvents()

	// 替换当前数据
	h.currentAgent = ""
	h.currentEvents = make([]MessageEvent, 0)
	h.currentRunPath = ""
	h.currentMemberID = ""
	h.currentMemberTool = ""
	h.currentMemberName = ""
	h.pendingToolCalls = nil
	h.toolResultChunks = make(map[string]string)

	return nil
}

func (h *HistoryRecorder) SaveToMarkdownFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 包含构建中的消息
	messages := make([]AgentMessage, len(h.messages))
	copy(messages, h.messages)

	if h.currentAgent != "" && len(h.currentEvents) > 0 {
		msg := AgentMessage{
			AgentName:      h.currentAgent,
			RunPath:        h.currentRunPath,
			MemberCallID:   h.currentMemberID,
			MemberToolName: h.currentMemberTool,
			MemberName:     h.currentMemberName,
			IsMemberEvent:  h.currentMemberID != "",
			StartTime:      h.currentStartTime,
			EndTime:        time.Now(),
			Events:         h.currentEvents,
		}
		messages = append(messages, msg)
	}

	return saveMessagesToMarkdown(messages, filePath)
}

func saveMessagesToMarkdown(messages []AgentMessage, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	var md strings.Builder

	md.WriteString("# 对话历史\n\n")
	fmt.Fprintf(&md, "**生成时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&md, "**对话轮次**: %d\n\n", len(messages))

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
		fmt.Fprintf(&md, "%s (%d次)", agent, count)
		first = false
	}
	md.WriteString("\n\n---\n\n")

	// 对话内容
	for i, msg := range messages {
		fmt.Fprintf(&md, "## %d. %s\n\n", i+1, msg.AgentName)

		duration := msg.EndTime.Sub(msg.StartTime)
		fmt.Fprintf(&md, "**时间**: %s - %s (%v)\n\n",
			msg.StartTime.Format("15:04:05"),
			msg.EndTime.Format("15:04:05"),
			duration.Round(time.Millisecond))

		if msg.RunPath != "" {
			fmt.Fprintf(&md, "**路径**: `%s`\n\n", msg.RunPath)
		}

		// 事件内容
		md.WriteString("**内容**:\n\n")
		for _, event := range msg.Events {
			switch event.Type {
			case MsgTypeText:
				md.WriteString(event.Content)
				md.WriteString("\n\n")

			case MsgTypeToolCall:
				if event.ToolCall != nil {
					display := FormatToolDisplay(event.ToolCall.Name)
					fmt.Fprintf(&md, "> **工具调用**: %s\n", display.DisplayName)
					if event.ToolCall.Arguments != "" {
						fmt.Fprintf(&md, "> - **参数**: `%s`\n", event.ToolCall.Arguments)
					}
					if event.ToolCall.Result != "" {
						fmt.Fprintf(&md, "> - **结果**: %s\n", event.ToolCall.Result)
					}
					md.WriteString("\n")
				}

			case MsgTypeAction:
				if event.Action != nil {
					fmt.Fprintf(&md, "> **[Action]**: [%s] %s\n\n", event.Action.ActionType, event.Action.Content)
				}

			case MsgTypeError:
				fmt.Fprintf(&md, "> **[错误]**: %s\n\n", event.Content)
			}
		}

		// 分隔线（除了最后一条消息）
		if i < len(messages)-1 {
			md.WriteString("---\n\n")
		}
	}

	if err := os.WriteFile(filePath, []byte(md.String()), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (h *HistoryRecorder) SaveToMarkdownWithTimestamp() (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(common.AppDir(), "history", "output_history", fmt.Sprintf("chat_%s.md", timestamp))
	err := h.SaveToMarkdownFile(filePath)
	return filePath, err
}

// CLIEventCallback 创建 CLI 模式的事件回调，同时记录和打印
func CLIEventCallback(recorder *HistoryRecorder) func(Event) error {
	return func(event Event) error {
		recorder.RecordEvent(event)
		PrintEvent(event)
		return nil
	}
}

// JSONEventCallback 创建 JSON 格式的事件回调，将事件序列化为 JSON 输出
func JSONEventCallback(recorder *HistoryRecorder) func(Event) error {
	return func(event Event) error {
		recorder.RecordEvent(event)
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
}

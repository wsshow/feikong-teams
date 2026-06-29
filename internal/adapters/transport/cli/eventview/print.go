package eventview

import (
	"encoding/json"
	"fkteams/internal/adapters/storage/file/history"
	fktui "fkteams/internal/adapters/transport/cli/tui"
	"fkteams/internal/app/agent/catalog/toolmeta"
	domainevent "fkteams/internal/domain/event"
	domainmessage "fkteams/internal/domain/message"
	runtimeevents "fkteams/internal/runtime/events"
	"fmt"

	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

const agentToolPrefix = toolmeta.AgentToolPrefix

type Event = domainevent.Event

const (
	EventAssistantText      = domainevent.TypeAssistantText
	EventAssistantReason    = domainevent.TypeAssistantReasoning
	EventToolCallArgs       = domainevent.TypeToolCallArguments
	EventAssistantCompleted = domainevent.TypeAssistantCompleted
	EventToolCallStarted    = domainevent.TypeToolCallStarted
	EventToolCallResult     = domainevent.TypeToolCallResult
	EventToolCallCompleted  = domainevent.TypeToolCallCompleted
	EventSystemNotice       = domainevent.TypeSystemNotice
	EventUsageReported      = domainevent.TypeUsageReported
	EventError              = domainevent.TypeError
)

func isInternalToolName(name string) bool {
	return runtimeevents.IsInternalToolName(name)
}

func isInternalContinueContent(content string) bool {
	return runtimeevents.IsInternalContinueContent(content)
}

func FormatToolDisplay(name string) toolmeta.ToolDisplay {
	return toolmeta.FormatToolDisplay(name)
}

var (
	PrintEvent      func(Event)
	FlushPrintEvent func() // 刷新流式缓冲
)

func init() {
	PrintEvent, FlushPrintEvent = newPrintEvent()
}

// CLIEventCallback 创建 CLI 模式的事件回调，同时记录和打印。
func CLIEventCallback(recorder *eventlog.HistoryRecorder) func(Event) error {
	return func(event Event) error {
		recorder.RecordEvent(event)
		PrintEvent(event)
		return nil
	}
}

// JSONEventCallback 创建 JSON 格式的事件回调，将事件序列化为 JSON 输出。
func JSONEventCallback(recorder *eventlog.HistoryRecorder) func(Event) error {
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

func newPrintEvent() (func(Event), func()) {
	agentName := ""
	lastToolName := ""
	toolNamesByID := map[string]string{}
	toolFlows := map[string]*terminalToolFlow{}
	toolPanel := fktui.NewToolPanel()
	toolPending := map[string]bool{}
	memberPanel := fktui.NewMemberPanel()
	activePanel := ""
	memberNamesByToolID := map[string]string{}
	memberKeysByToolID := map[string]string{}
	memberPending := map[string]bool{}
	memberStarted := map[string]bool{}
	memberResultChunks := map[string]string{}
	var deferredEvents []Event
	replayingDeferred := false
	inReasoning := false
	var sb streamBuf
	var rw *reasoningWriter

	tryFlush := func() {
		if sb.buf.Len() > 0 {
			sb.flush()
		}
	}

	finishToolPanel := func() {
		toolPanel.Finish()
		if activePanel == "tool" {
			activePanel = ""
		}
	}

	finishMemberPanel := func() {
		memberPanel.Finish()
		if activePanel == "member" {
			activePanel = ""
		}
	}

	activateMemberPanel := func() {
		if activePanel == "tool" {
			finishToolPanel()
			toolPending = map[string]bool{}
			toolFlows = map[string]*terminalToolFlow{}
		} else if activePanel == "" {
			fmt.Println()
		}
		activePanel = "member"
	}

	sendMemberPanel := func(e fktui.MemberEvent) bool {
		activateMemberPanel()
		return memberPanel.Send(e)
	}

	activateToolPanel := func() {
		if activePanel == "member" {
			finishMemberPanel()
		} else if activePanel == "" {
			fmt.Println()
		}
		activePanel = "tool"
	}

	ensureMember := func(key, name string) {
		if key == "" {
			return
		}
		if name == "" {
			name = agentDisplayName(key)
		}
		if !memberStarted[key] {
			memberStarted[key] = true
			memberPending[key] = true
			sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "start"})
			return
		}
		sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "meta"})
	}

	memberFromEvent := func(event Event) (string, string) {
		key := event.MemberCallID
		if key == "" {
			key = agentKey(event.AgentName)
		}
		name := event.MemberName
		if name == "" {
			name = agentDisplayName(event.AgentName)
		}
		ensureMember(key, name)
		return key, name
	}

	memberToolKey := func(event Event, tool domainmessage.ToolCall, position int) string {
		if ref := runtimeevents.ToolCallRefAt(event, tool, position); ref != "" {
			return "ref:" + ref
		}
		return ""
	}

	regularToolKey := func(event Event, tool domainmessage.ToolCall, position int) string {
		if ref := runtimeevents.ToolCallRefAt(event, tool, position); ref != "" {
			return "ref:" + ref
		}
		return ""
	}

	ensureToolFlow := func(key, name string) *terminalToolFlow {
		if key == "" {
			return nil
		}
		flow := toolFlows[key]
		if flow == nil {
			flow = &terminalToolFlow{Key: key, Name: name, Status: "参数准备中"}
			toolFlows[key] = flow
		}
		if name != "" {
			flow.Name = name
		}
		return flow
	}

	sendToolPanel := func(key string, flow *terminalToolFlow, eventType string, content string, appendContent bool) bool {
		if flow == nil {
			return false
		}
		name := flow.Name
		if name == "" {
			return false
		}
		panelType := eventType
		switch eventType {
		case "content":
			panelType = "result"
		case "op":
			panelType = "args"
		}
		activateToolPanel()
		sent := toolPanel.Send(fktui.ToolEvent{
			Key:     key,
			Name:    name,
			Type:    panelType,
			Content: content,
			Append:  appendContent,
		})
		if sent {
			toolPending[key] = eventType != "done" && eventType != "error"
		}
		return sent
	}

	finishToolsIfIdle := func() {
		for _, pending := range toolPending {
			if pending {
				return
			}
		}
		finishToolPanel()
		toolPending = map[string]bool{}
		toolFlows = map[string]*terminalToolFlow{}
	}

	printToolFlow := func(flow *terminalToolFlow) {
		if flow == nil {
			return
		}
		name := flow.Name
		if name == "" {
			name = lastToolName
		}
		display := FormatToolDisplay(name)
		title := display.DisplayName
		if title == "" {
			title = name
		}
		fmt.Printf("\n\033[1;35m[%s] 工具: \033[1m%s\033[0m \033[90m(%s)\033[0m\n", flow.AgentName, title, flow.Status)
		if flow.Args != "" {
			fmt.Printf("  参数: %s\n", truncateString(flow.Args, 240))
		}
		if flow.Result != "" {
			fmt.Printf("  结果:\n")
			formatted := formatToolResultForPrint(name, flow.Result)
			if formatted != "" {
				fmt.Print(formatted)
			} else {
				printPlainResult(flow.Result)
			}
		}
		fmt.Println()
	}

	hasPendingMembers := func() bool {
		if len(memberPending) == 0 {
			return false
		}
		for _, pending := range memberPending {
			if pending {
				return true
			}
		}
		return false
	}

	var printFn func(Event)

	resetMemberState := func() {
		memberPending = map[string]bool{}
		memberStarted = map[string]bool{}
		memberNamesByToolID = map[string]string{}
		memberKeysByToolID = map[string]string{}
		memberResultChunks = map[string]string{}
	}

	finalizeChunkedMemberResults := func() {
		for callID, content := range memberResultChunks {
			key := memberKeysByToolID[callID]
			if key == "" || !memberPending[key] {
				delete(memberResultChunks, callID)
				continue
			}
			memberName := memberNamesByToolID[callID]
			if isErrorContent(content) {
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "error", Content: content, ToolName: memberName})
			} else {
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "done"})
			}
			memberPending[key] = false
			delete(memberResultChunks, callID)
		}
	}

	finishMembersIfIdle := func() {
		finalizeChunkedMemberResults()
		if hasPendingMembers() {
			return
		}
		finishMemberPanel()
		resetMemberState()
		if len(deferredEvents) > 0 && !replayingDeferred {
			events := deferredEvents
			deferredEvents = nil
			replayingDeferred = true
			for _, deferred := range events {
				printFn(deferred)
			}
			replayingDeferred = false
		}
	}

	flushDeferred := func() {
		finalizeChunkedMemberResults()
		finishMemberPanel()
		resetMemberState()
		if len(deferredEvents) > 0 && !replayingDeferred {
			events := deferredEvents
			deferredEvents = nil
			replayingDeferred = true
			for _, deferred := range events {
				printFn(deferred)
			}
			replayingDeferred = false
		}
	}

	finishMembersBeforeParentOutput := func(event Event) bool {
		if len(memberPending) == 0 || isMemberEvent(event) {
			return false
		}
		finalizeChunkedMemberResults()
		if hasPendingMembers() && !replayingDeferred {
			deferredEvents = append(deferredEvents, event)
			return true
		}
		finishMemberPanel()
		resetMemberState()
		return false
	}

	registerAgentToolCall := func(tool domainmessage.ToolCall) (string, string) {
		_, memberName, _ := agentToolKey(tool.Function.Name)
		key := tool.ID
		if tool.ID != "" {
			memberNamesByToolID[tool.ID] = memberName
			memberKeysByToolID[tool.ID] = key
			toolNamesByID[tool.ID] = tool.Function.Name
		}
		ensureMember(key, memberName)
		return key, memberName
	}

	splitAgentToolCalls := func(toolCalls []domainmessage.ToolCall) (agents, others []domainmessage.ToolCall) {
		for _, tool := range toolCalls {
			if isInternalToolName(tool.Function.Name) {
				continue
			}
			display := FormatToolDisplay(tool.Function.Name)
			if display.Kind == "agent" {
				agents = append(agents, tool)
			} else {
				others = append(others, tool)
			}
		}
		return agents, others
	}

	printFn = func(event Event) {
		switch event.Type {
		case EventAssistantText, EventAssistantReason, EventToolCallArgs:
			switch event.DeltaKind {
			case domainevent.DeltaReasoning:
				if isMemberEvent(event) {
					key, name := memberFromEvent(event)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.Content})
					return
				}
				if finishMembersBeforeParentOutput(event) {
					return
				}
				tryFlush()
				if agentName != event.AgentName {
					agentName = event.AgentName
					fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
				}
				if !inReasoning {
					inReasoning = true
					rw = newReasoningWriter()
					fmt.Printf("%s\033[90m[思考] \033[0m%s", reasoningPrefix, "\033[3;90m")
					rw.col = 6
				}
				rw.writeChunk(event.Content)
				return

			case domainevent.DeltaToolArgs:
				if isMemberEvent(event) {
					key, name := memberFromEvent(event)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_args", ToolKey: memberToolKey(event, domainmessage.ToolCall{}, 0), ToolName: event.ToolName, Content: event.Content, Append: true})
					return
				}
				toolName := event.ToolName
				if toolName == "" && event.ToolCallID != "" {
					toolName = toolNamesByID[event.ToolCallID]
				}
				if toolName != "" {
					if key, memberName, ok := agentToolKey(toolName); ok {
						if event.ToolCallID != "" {
							if mapped := memberKeysByToolID[event.ToolCallID]; mapped != "" {
								key = mapped
							} else {
								key = event.ToolCallID
								memberKeysByToolID[event.ToolCallID] = key
							}
							if memberNamesByToolID[event.ToolCallID] == "" {
								memberNamesByToolID[event.ToolCallID] = memberName
							}
						}
						if key == "" {
							return
						}
						sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "op", Content: "任务参数接收中"})
						return
					}
				}
				key := regularToolKey(event, domainmessage.ToolCall{}, 0)
				flow := ensureToolFlow(key, toolName)
				if flow == nil {
					return
				}
				flow.AgentName = event.AgentName
				flow.Status = "参数准备中"
				flow.Args += event.Content
				if toolName == "" {
					return
				}
				sendToolPanel(key, flow, "op", flow.Args, false)
				return

			default:
				if isMemberEvent(event) {
					key, name := memberFromEvent(event)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.Content})
					return
				}
				if finishMembersBeforeParentOutput(event) {
					return
				}
				wasReasoning := inReasoning
				if inReasoning {
					inReasoning = false
					fmt.Printf("\033[0m\n")
				}
				if agentName != event.AgentName {
					tryFlush()
					agentName = event.AgentName
					fmt.Printf("\n\033[1;36m╭─ [%s] %s\033[0m\n", agentName, event.RunPath)
					sb.agent = agentName
					sb.path = event.RunPath
				} else if wasReasoning && sb.agent == "" {
					sb.agent = agentName
					sb.path = event.RunPath
				}
				if sb.agent == "" {
					sb.agent = event.AgentName
					sb.path = event.RunPath
				}
				sb.addChunk(event.Content)
			}

		case EventAssistantCompleted:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				if event.ReasoningContent != "" {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.ReasoningContent})
				}
				if event.Content != "" {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "content", Content: event.Content})
				}
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			printContent := event.Content
			if event.Content != "" && sameStreamMessage(sb.content(), event.Content) {
				sb.discard()
				printContent = ""
			} else {
				tryFlush()
			}
			if inReasoning {
				inReasoning = false
				fmt.Printf("\033[0m\n")
			}
			if event.ReasoningContent != "" {
				fmt.Printf("\n\033[90m[%s] 思考:\033[0m \033[3;90m%s\033[0m\n", event.AgentName, event.ReasoningContent)
			}
			if printContent != "" {
				fmt.Printf("\n\033[1;32m✓ [%s]\033[0m\n", event.AgentName)
				lipgloss.Println(formatAssistantOutput(fktui.RenderMarkdown(printContent)))
			}

		case EventToolCallCompleted:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				if event.Content != "" {
					toolKey := memberToolKey(event, domainmessage.ToolCall{}, 0)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_result", ToolKey: toolKey, ToolName: event.ToolName, Content: event.Content})
				}
				return
			}
			toolName := lastToolName
			if event.ToolName != "" {
				toolName = event.ToolName
			}
			if event.ToolCallID != "" {
				if name, ok := toolNamesByID[event.ToolCallID]; ok {
					toolName = name
				}
			}
			if isInternalToolName(toolName) || isInternalContinueContent(event.Content) {
				return
			}
			display := FormatToolDisplay(toolName)
			if display.Kind == "agent" {
				_, memberName, _ := agentToolKey(toolName)
				if event.ToolCallID != "" && memberNamesByToolID[event.ToolCallID] != "" {
					memberName = memberNamesByToolID[event.ToolCallID]
				}
				key := ""
				if event.ToolCallID != "" && memberKeysByToolID[event.ToolCallID] != "" {
					key = memberKeysByToolID[event.ToolCallID]
				}
				if key == "" {
					return
				}
				if isErrorContent(event.Content) {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "error", Content: event.Content, ToolKey: memberToolKey(event, domainmessage.ToolCall{}, 0), ToolName: toolName})
				} else {
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "done"})
				}
				memberPending[key] = false
				finishMembersIfIdle()
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			key := regularToolKey(event, domainmessage.ToolCall{}, 0)
			flow := ensureToolFlow(key, toolName)
			if flow == nil {
				return
			}
			flow.AgentName = event.AgentName
			flow.Status = "已完成"
			if event.Content != "" {
				if flow.Result != "" && !strings.Contains(flow.Result, event.Content) {
					flow.Result += event.Content
				} else if flow.Result == "" {
					flow.Result = event.Content
				}
			}
			doneContent := flow.Result
			if flow.Streamed {
				doneContent = ""
			}
			sent := sendToolPanel(key, flow, "done", doneContent, false)
			finishToolsIfIdle()
			if !sent {
				printToolFlow(flow)
			}
			delete(toolFlows, key)

		case EventToolCallResult:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				toolKey := memberToolKey(event, domainmessage.ToolCall{}, 0)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_result", ToolKey: toolKey, ToolName: event.ToolName, Content: event.Content, Append: true})
				return
			}
			if event.ToolCallID != "" && memberKeysByToolID[event.ToolCallID] != "" {
				memberResultChunks[event.ToolCallID] += event.Content
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			key := regularToolKey(event, domainmessage.ToolCall{}, 0)
			flow := ensureToolFlow(key, event.ToolName)
			if flow == nil {
				return
			}
			flow.AgentName = event.AgentName
			flow.Status = "执行中"
			flow.Result += event.Content
			flow.Streamed = true
			sendToolPanel(key, flow, "content", event.Content, true)

		case EventToolCallStarted:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				for i, tool := range runtimeevents.ToolCallsFromEvent(event) {
					if tool.Function.Name == "" {
						continue
					}
					display := FormatToolDisplay(tool.Function.Name)
					sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "tool_args", ToolKey: memberToolKey(event, tool, i), ToolName: display.DisplayName, Content: tool.Function.Arguments})
				}
				return
			}
			agentTools, otherTools := splitAgentToolCalls(runtimeevents.ToolCallsFromEvent(event))
			if len(agentTools) > 0 {
				if inReasoning {
					inReasoning = false
					fmt.Printf("\033[0m\n")
				}
				tryFlush()
			}
			for _, tool := range agentTools {
				key, memberName := registerAgentToolCall(tool)
				if key == "" {
					continue
				}
				op := "任务已分配"
				if tool.Function.Arguments != "" {
					op = "任务: " + tool.Function.Arguments
				}
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: memberName, Type: "op", Content: op})
			}
			if len(agentTools) > 0 && len(otherTools) == 0 {
				return
			}
			if len(agentTools) > 0 {
				event.ToolCalls = otherTools
				if hasPendingMembers() && !replayingDeferred {
					deferredEvents = append(deferredEvents, event)
					return
				}
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			toolCalls := runtimeevents.ToolCallsFromEvent(event)
			for i, tool := range toolCalls {
				if isInternalToolName(tool.Function.Name) {
					continue
				}
				if tool.ID != "" {
					toolNamesByID[tool.ID] = tool.Function.Name
				}
				key := regularToolKey(event, tool, i)
				flow := ensureToolFlow(key, tool.Function.Name)
				if flow == nil {
					continue
				}
				flow.AgentName = event.AgentName
				flow.Status = "已调用"
				if tool.Function.Arguments != "" {
					flow.Args = tool.Function.Arguments
					sendToolPanel(key, flow, "op", tool.Function.Arguments, false)
				}
				if i == len(toolCalls)-1 {
					lastToolName = tool.Function.Name
				}
			}

		case EventSystemNotice:
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			if event.Content != "" {
				fmt.Printf("\n\033[1;33m✓ [%s] %s\033[0m\n", event.AgentName, event.Content)
			}

		case EventUsageReported:
			return

		case EventError:
			if isMemberEvent(event) {
				key, name := memberFromEvent(event)
				sendMemberPanel(fktui.MemberEvent{Key: key, Name: name, Type: "error", Content: event.Error})
				memberPending[key] = false
				finishMembersIfIdle()
				return
			}
			if finishMembersBeforeParentOutput(event) {
				return
			}
			tryFlush()
			fmt.Printf("\n\033[1;31m✗ [%s] 错误:\033[0m\n", event.AgentName)
			fmt.Printf("  \033[31m%s\033[0m\n", event.Error)
			if event.RunPath != "" {
				fmt.Printf("  路径: %s\n", event.RunPath)
			}
			fmt.Println()

		default:
			if finishMembersBeforeParentOutput(event) {
				return
			}
			fmt.Printf("\n\033[1;90m? 未知事件: %s\033[0m\n", event.Type)
			if event.AgentName != "" {
				fmt.Printf("  代理: %s\n", event.AgentName)
			}
			if event.Content != "" {
				fmt.Printf("  内容: %s\n", event.Content)
			}
		}
	}

	return printFn, func() {
		tryFlush()
		flushDeferred()
	}
}

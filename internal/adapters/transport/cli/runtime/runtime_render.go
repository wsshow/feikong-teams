package runtime

import (
	"fkteams/internal/adapters/transport/cli/tui"

	"fmt"

	"strconv"
	"strings"

	"time"

	tea "charm.land/bubbletea/v2"
)

func (m runtimeModel) View() tea.View {
	content := m.screenContent()
	if m.selection.Active {
		content = m.renderSelection(content)
	}
	content = m.renderFloatingCopiedNotice(content)
	content = tui.RenderRuntimeScreen(content, m.screenWidth(), m.viewHeight(), runtimeHorizontalGutter)
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m runtimeModel) screenContent() string {
	bottom := m.renderBottom()
	bottomLines := tui.LineCount(bottom)
	available := m.bodyHeightForBottom(bottomLines)
	body := m.renderVisibleTranscript(available)
	var sb strings.Builder
	if body != "" {
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteString("\n")
		}
	}
	bodyLines := tui.LineCount(body)
	for i := bodyLines; i < available; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(bottom)
	return sb.String()
}

func (m runtimeModel) renderSelection(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = m.selection.RenderLine(i, line)
	}
	return strings.Join(lines, "\n")
}

func (m runtimeModel) renderFloatingCopiedNotice(content string) string {
	if !m.isCopiedNoticeVisible() {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	notice := tui.CopiedNotice(m.selection.Copied)
	rendered := tui.CenterLine(tui.Dim(notice), m.contentWidth())
	target := max(0, len(lines)-tui.LineCount(m.renderInputBox())-1)
	lines[target] = rendered
	return strings.Join(lines, "\n")
}

func (m runtimeModel) mouseTextPoint(mouse tea.Mouse) tui.TextPoint {
	return tui.TextPoint{
		X: max(0, mouse.X-runtimeHorizontalGutter),
		Y: min(mouse.Y, max(0, m.viewHeight()-1)),
	}
}

func (m runtimeModel) hitMemberSummary(mouse tea.Mouse) string {
	if mouse.Y < 0 || mouse.Y >= m.viewHeight() {
		return ""
	}
	lines := m.screenLines()
	if mouse.Y >= len(lines) {
		return ""
	}
	line := tui.StripANSI(lines[mouse.Y])
	start := strings.Index(line, "[")
	end := strings.Index(line, "]")
	if start < 0 || end <= start+1 || !strings.Contains(line[:start], "›") {
		return ""
	}
	ordinal, err := strconv.Atoi(strings.TrimSpace(line[start+1 : end]))
	if err != nil || ordinal <= 0 {
		return ""
	}
	return m.memberKeyByOrdinal(ordinal)
}

func (m runtimeModel) hitJumpToBottom(mouse tea.Mouse) bool {
	if m.currentScrollOffset() <= 0 {
		return false
	}
	y, startX, endX := m.jumpToBottomBounds()
	return mouse.Y == y && mouse.X >= startX && mouse.X < endX
}

func (m runtimeModel) jumpToBottomBounds() (int, int, int) {
	label := tui.StripANSI(tui.JumpToBottomButton())
	labelWidth := tui.CellWidth(label)
	for y, line := range strings.Split(m.screenContent(), "\n") {
		x := strings.Index(tui.StripANSI(line), label)
		if x >= 0 {
			startX := runtimeHorizontalGutter + x
			return y, startX, startX + labelWidth
		}
	}
	return -1, -1, -1
}

func (m runtimeModel) screenLines() []string {
	return strings.Split(m.screenContent(), "\n")
}

func (m runtimeModel) viewHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	return height
}

func (m runtimeModel) bodyHeightForBottom(bottomLines int) int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	available := height - bottomLines
	if available < 0 {
		return 0
	}
	return available
}

func (m runtimeModel) visibleTranscriptLines(maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	transcript := strings.TrimRight(m.transcriptText(), "\n")
	if transcript == "" {
		return nil
	}
	return strings.Split(tui.VisibleLines(transcript, maxLines, m.currentScrollOffset()), "\n")
}

func (m runtimeModel) renderVisibleTranscript(maxLines int) string {
	lines := m.visibleTranscriptLines(maxLines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (m runtimeModel) selectedVisibleText() string {
	lines := m.screenLines()
	return m.selection.SelectedText(lines)
}

func (m runtimeModel) transcriptText() string {
	if member := m.currentMember(); member != nil {
		header := tui.Banner("成员详情: "+member.Name) + "\n" + tui.Dim("Esc / Backspace 返回主界面")
		if member.Task != "" {
			header += "\n" + tui.Dim("目标: "+truncateRuntimeText(member.Task, max(20, m.contentWidth()-6)))
		}
		body := m.memberBlocksText(member)
		if strings.TrimSpace(body) == "" {
			return header
		}
		return header + "\n\n" + body
	}
	return m.blocksText(m.blocks)
}

func (m runtimeModel) memberBlocksText(member *runtimeMemberState) string {
	if member == nil {
		return ""
	}
	if !member.RenderDirty && member.RenderCache != "" {
		return member.RenderCache
	}
	var transcript strings.Builder
	for i, block := range member.Blocks {
		if i > 0 && shouldSpaceBeforeBlock(member.Blocks[i-1].Kind, block.Kind) {
			transcript.WriteString("\n")
		}
		fmt.Fprintf(&transcript, "%s\n", m.renderMemberBlock(member, block))
	}
	member.RenderCache = tui.WrapLines(transcript.String(), m.contentWidth())
	member.RenderDirty = false
	return member.RenderCache
}

func (m runtimeModel) renderMemberBlock(member *runtimeMemberState, block runtimeBlock) string {
	if block.Kind == runtimeBlockTool {
		rendered := runtimeRenderToolBlock(block)
		if askState := member.askForToolKey(block.ToolKey); askState != nil {
			if m.memberView == member.Key && !askState.Answered {
				return rendered
			}
			rendered += "\n" + m.renderAskPanel(*askState)
		}
		return rendered
	}
	return m.renderBlock(block)
}

func (m runtimeModel) blocksText(blocks []runtimeBlock) string {
	var transcript strings.Builder
	for i, block := range blocks {
		if i > 0 && shouldSpaceBeforeBlock(blocks[i-1].Kind, block.Kind) {
			transcript.WriteString("\n")
		}
		fmt.Fprintf(&transcript, "%s\n", m.renderBlock(block))
	}
	return tui.WrapLines(transcript.String(), m.contentWidth())
}

func shouldSpaceBeforeBlock(prev runtimeBlockKind, current runtimeBlockKind) bool {
	switch current {
	case runtimeBlockUser, runtimeBlockReasoning, runtimeBlockDone:
		return true
	case runtimeBlockSystem, runtimeBlockError:
		return prev == runtimeBlockUser
	default:
		return false
	}
}

func (m runtimeModel) bodyHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	available := height - tui.LineCount(m.renderBottom())
	if available < 0 {
		return 0
	}
	return available
}

func (m runtimeModel) renderBottom() string {
	var sb strings.Builder
	statusStarted := false
	writeStatusLine := func(line string) {
		if !statusStarted {
			if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			statusStarted = true
		}
		fmt.Fprintf(&sb, "%s\n", line)
	}
	if m.picker != nil {
		sb.WriteString(m.renderPicker())
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
	}
	if m.ask != nil {
		sb.WriteString(m.renderAskPanel(*m.ask))
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
	}
	if askState := m.currentMemberPendingAsk(); askState != nil {
		sb.WriteString(m.renderAskPanel(*askState))
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
	}
	if m.approval != nil {
		sb.WriteString(m.renderApprovalPanel())
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
	}
	if m.currentScrollOffset() > 0 {
		writeStatusLine(tui.CenterLine(tui.JumpToBottomButton(), m.contentWidth()))
	}
	if m.running {
		writeStatusLine(tui.Status(m.status))
	}
	if m.isExitConfirming() {
		seconds := int(time.Until(m.exitUntil).Seconds())
		if seconds < 1 {
			seconds = 1
		}
		writeStatusLine(tui.Dim("再按 ") + tui.Key("Ctrl+C") + tui.Dim(" 退出 · ") + tui.Dim(fmt.Sprintf("%ds", seconds)))
	}
	if tokenStatus := m.tokenStatus(); tokenStatus != "" {
		fmt.Fprintf(&sb, "%s\n", tui.RightLine(tui.Dim(tokenStatus), m.contentWidth()))
	}
	sb.WriteString(m.renderInputBox())
	return sb.String()
}

func (m runtimeModel) renderInputBox() string {
	if m.memberView != "" {
		return tui.RenderRuntimeInputBox(max(24, m.contentWidth()), m.renderMemberDetailInputValue(), m.memberDetailHint())
	}
	content := m.renderInputValue()
	return tui.RenderRuntimeInputBox(max(24, m.contentWidth()), content, m.inputHint())
}

func (m runtimeModel) renderMemberDetailInputValue() string {
	if m.currentMemberPendingAsk() != nil {
		return m.renderInputValue()
	}
	return tui.Dim("当前为成员详情，返回主界面后继续输入")
}

func (m runtimeModel) renderInputValue() string {
	value := m.input.Value()
	rendered := tui.PromptMarker() + tui.RenderInlineInputValueAtCursor(value, m.input.Position())
	if hint := runtimeCommandUsageHint(value, m.input.Position()); hint != "" {
		rendered += tui.Dim(hint)
	}
	return rendered
}

func (m runtimeModel) memberDetailHint() string {
	if askState := m.currentMemberPendingAsk(); askState != nil {
		hints := []string{"回答当前成员 ask", "Enter 提交", "Esc 返回"}
		if len(askState.Options) > 0 {
			hints = append(hints, "可输入序号")
		}
		if askState.MultiSelect {
			hints = append(hints, "多选逗号分隔")
		}
		return strings.Join(hints, " · ")
	}
	return strings.Join([]string{
		"成员详情",
		"Esc/Backspace 返回",
		"↑↓/PgUp/PgDn 滚动",
		"End 到底部",
		"Ctrl+C 退出",
	}, " · ")
}

func (m runtimeModel) inputHint() string {
	if m.ask != nil {
		hints := []string{"回答提问", "↑↓ 选择", "Enter 提交", "Esc 取消", "Ctrl+C 取消任务"}
		if len(m.ask.Options) == 0 {
			hints = []string{"回答提问", "输入回答", "Enter 提交", "Esc 取消", "Ctrl+C 取消任务"}
		}
		if m.ask.MultiSelect {
			hints = []string{"回答提问", "多选逗号分隔", "Enter 提交", "Esc 取消", "Ctrl+C 取消任务"}
		}
		return strings.Join(hints, " · ")
	}
	if m.approval != nil {
		return strings.Join([]string{
			"权限审批",
			"↑↓ 选择",
			"Enter 确认",
			"Esc 拒绝",
			"Ctrl+C 取消任务",
		}, " · ")
	}
	if m.running {
		return strings.Join([]string{
			runtimeModeName(m.runtime.session.CurrentMode),
			"Enter 转向",
			"Esc 暂停并取回转向",
			"Ctrl+C 取消",
		}, " · ")
	}
	return strings.Join([]string{
		runtimeModeName(m.runtime.session.CurrentMode),
		"@ 智能体",
		"# 文件",
		"/ 命令",
	}, " · ")
}

func (m runtimeModel) tokenStatus() string {
	if m.totalTokens <= 0 {
		return ""
	}
	return fmt.Sprintf("%d tokens", m.totalTokens)
}

func (m runtimeModel) renderPicker() string {
	p := m.picker
	if p == nil {
		return ""
	}
	var sb strings.Builder
	title := p.title
	if p.kind == runtimePickerFile {
		displayDir := p.currentRel()
		if displayDir == "." {
			displayDir = "工作目录"
		}
		title = fmt.Sprintf("%s [%s]", p.title, displayDir)
	}
	fmt.Fprintf(&sb, "%s\n", tui.PickerTitle("? "+title))
	if p.filter != "" {
		fmt.Fprintf(&sb, "%s\n", tui.Status("  / "+p.filter))
	}
	if len(p.matches) == 0 {
		fmt.Fprintf(&sb, "%s\n", tui.Dim("  (无匹配项)"))
	} else {
		end := min(p.offset+p.height, len(p.matches))
		if p.offset > 0 {
			fmt.Fprintf(&sb, "%s\n", tui.Dim("  ↑ 更多..."))
		}
		for i := p.offset; i < end; i++ {
			item := p.items[p.matches[i]]
			if i == p.cursor {
				fmt.Fprintf(&sb, "%s\n", tui.PickerSelected("  > "+item.Label))
			} else {
				fmt.Fprintf(&sb, "    %s\n", item.Label)
			}
		}
		if end < len(p.matches) {
			fmt.Fprintf(&sb, "%s\n", tui.Dim("  ↓ 更多..."))
		}
	}
	fmt.Fprintf(&sb, "%s", tui.Dim("  ↑↓ 移动 | Enter 选择 | Esc 返回 | 输入过滤"))
	return tui.PickerBox(max(20, m.contentWidth()), sb.String())
}

func (m runtimeModel) renderBlock(block runtimeBlock) string {
	switch block.Kind {
	case runtimeBlockUser:
		return tui.RenderUserMessageBlock(block.Content, m.contentWidth())
	case runtimeBlockWelcome:
		return tui.RenderWelcomePanel(m.welcome, m.contentWidth())
	case runtimeBlockReasoning:
		return tui.Reasoning(block.Content) + "\n"
	case runtimeBlockError:
		return tui.Error(block.Title + " " + block.Content)
	case runtimeBlockDone:
		return tui.DoneMarker() + tui.Dim(fmt.Sprintf("Worked for %s", block.Content))
	case runtimeBlockMeta:
		return tui.Dim(fmt.Sprintf("%s ID: %s", block.Title, block.Content))
	case runtimeBlockBanner:
		return tui.Banner(fmt.Sprintf("%s: %s", block.Title, block.Content))
	case runtimeBlockInterrupt:
		return tui.Interrupted(block.Content)
	case runtimeBlockMember:
		return m.renderMemberSummary(block)
	case runtimeBlockSystem:
		return tui.System(block.Title) + "\n" + m.runtimeRenderMarkdown(block.Content)
	case runtimeBlockTool:
		return runtimeRenderToolBlock(block)
	default:
		return m.runtimeRenderMarkdown(block.Content)
	}
}

func (m runtimeModel) renderMemberSummary(block runtimeBlock) string {
	ordinal := m.memberOrdinal(block.MemberKey)
	status := runtimeMemberStatusText(block.MemberStatus)
	line := fmt.Sprintf("› [%d] %s  %s · 工具 %d · Enter/点击查看",
		ordinal,
		emptyRuntimeMemberName(block.MemberName),
		status,
		block.MemberTools,
	)
	if block.MemberStatus == "running" || block.MemberStatus == "waiting" || block.MemberStatus == "error" {
		if member := m.members[block.MemberKey]; member != nil {
			for _, toolLine := range tui.RenderToolChainLines(runtimeMemberToolChainItems(member), max(20, m.contentWidth()-4)) {
				line += "\n" + tui.Dim(toolLine)
			}
		}
	}
	switch block.MemberStatus {
	case "done":
		return tui.System(line)
	case "error":
		return tui.Error(line)
	default:
		return tui.Status(line)
	}
}

func runtimeMemberStatusText(status string) string {
	switch status {
	case "done":
		return "已完成"
	case "error":
		return "失败"
	case "waiting":
		return "等待用户回答"
	case "running":
		return "运行中"
	default:
		return "等待中"
	}
}

func (m runtimeModel) memberOrdinal(key string) int {
	ordinal := 0
	for _, block := range m.blocks {
		if block.Kind != runtimeBlockMember {
			continue
		}
		ordinal++
		if block.MemberKey == key {
			return ordinal
		}
	}
	return ordinal + 1
}

func (m runtimeModel) memberKeyByOrdinal(ordinal int) string {
	current := 0
	for _, block := range m.blocks {
		if block.Kind != runtimeBlockMember {
			continue
		}
		current++
		if current == ordinal {
			return block.MemberKey
		}
	}
	return ""
}

func runtimeRenderToolBlock(block runtimeBlock) string {
	if block.ToolName == "" {
		if block.Content != "" {
			return block.Content
		}
		block.ToolName = runtimeDefaultToolName
	}
	if block.ToolHasResult {
		return tui.ToolResult(block.ToolName, block.ToolArgs, block.ToolResult, block.ToolStatus)
	}
	return tui.ToolCall(block.ToolName, block.ToolArgs, block.ToolStatus)
}

func (m runtimeModel) renderAskPanel(askState runtimeAskState) string {
	var sb strings.Builder
	if askState.Question != "" {
		fmt.Fprintf(&sb, "  %s %s\n", tui.Dim("问题:"), askState.Question)
	}
	if len(askState.Options) > 0 {
		sb.WriteString(tui.Dim("  选项:") + "\n")
		for i, option := range askState.Options {
			prefix := "   "
			label := option
			if !askState.Answered && i == askState.SelectedIndex {
				prefix = " > "
				label = tui.PickerSelected(option)
			}
			fmt.Fprintf(&sb, "%s%d. %s\n", prefix, i+1, label)
		}
	}
	answer := runtimeAskResponseSummary(askState.Selected, askState.FreeText)
	if askState.Answered {
		if answer == "" {
			answer = "已提交"
		}
		fmt.Fprintf(&sb, "  %s %s", tui.Dim("已回答:"), answer)
		return strings.TrimRight(sb.String(), "\n")
	}
	hint := "在底部输入框回答后按 Enter 提交"
	if len(askState.Options) > 0 {
		hint = "输入选项序号或文本后按 Enter 提交"
		if askState.MultiSelect {
			hint = "多选用逗号分隔序号，或输入文本后按 Enter 提交"
		}
	}
	fmt.Fprintf(&sb, "  %s %s", tui.Status("等待回答:"), tui.Dim(hint))
	return strings.TrimRight(sb.String(), "\n")
}

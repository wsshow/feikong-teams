package tui

import (
	"strings"
	"testing"
)

func TestRuntimeLayoutHelpers(t *testing.T) {
	if PromptMarker() == "" || DoneMarker() == "" {
		t.Fatal("markers should not be empty")
	}
	if got := CenterLine("abc", 7); got != "  abc" {
		t.Fatalf("CenterLine = %q", got)
	}
	if got := RightLine("abc", 6); got != "   abc" {
		t.Fatalf("RightLine = %q", got)
	}

	screen := RenderRuntimeScreen("abcdef\nxy", 6, 3, 1)
	lines := strings.Split(screen, "\n")
	if len(lines) != 3 {
		t.Fatalf("screen lines = %d, want 3: %q", len(lines), screen)
	}
	for _, line := range lines {
		if CellWidth(StripANSI(line)) != 6 {
			t.Fatalf("screen line width = %d, want 6: %q", CellWidth(StripANSI(line)), line)
		}
	}

	input := StripANSI(RenderRuntimeInputBox(10, "hello world", "hint"))
	if !strings.Contains(input, "hello") || !strings.Contains(input, "hint") {
		t.Fatalf("runtime input box = %q", input)
	}
	user := StripANSI(RenderUserMessageBlock("hello\nworld", 20))
	if !strings.Contains(user, "hello") || !strings.Contains(user, "world") {
		t.Fatalf("user message block = %q", user)
	}
	userLines := strings.Split(user, "\n")
	if len(userLines) != 4 {
		t.Fatalf("user message lines = %d, want 4: %q", len(userLines), user)
	}
	for _, line := range userLines {
		if CellWidth(line) != 20 {
			t.Fatalf("user message line width = %d, want 20: %q", CellWidth(line), line)
		}
	}
	if strings.TrimSpace(strings.TrimPrefix(userLines[0], "▌")) != "" || strings.TrimSpace(strings.TrimPrefix(userLines[len(userLines)-1], "▌")) != "" {
		t.Fatalf("user message should include vertical padding with accent: %q", user)
	}
	if !strings.HasPrefix(userLines[0], "▌") || !strings.HasPrefix(userLines[1], "▌") {
		t.Fatalf("user message should render accent bar: %q", user)
	}
	if !strings.HasPrefix(userLines[1], "▌  hello") {
		t.Fatalf("user message content should keep compact left padding: %q", user)
	}
	if strings.Contains(user, PromptMarker()) {
		t.Fatalf("user message block should not render prompt marker: %q", user)
	}
}

func TestWelcomeAndStyledTextHelpers(t *testing.T) {
	panel := StripANSI(RenderWelcomePanel(WelcomeInfo{
		Version:   "v1",
		Mode:      "team",
		SessionID: "sid",
		Workspace: "/tmp/ws",
		Model:     "gpt",
	}, 60))
	for _, want := range []string{"非空小队", "team", "gpt", "/tmp/ws", "sid"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("welcome panel missing %q: %q", want, panel)
		}
	}
	if emptyAs("  ", "fallback") != "fallback" || emptyAs(" value ", "fallback") != "value" {
		t.Fatal("emptyAs returned unexpected value")
	}
	collapsedReasoning := StripANSI(ReasoningBlock("line 1\nline 2", true, "1.2s"))
	if !strings.Contains(collapsedReasoning, "Thought · 2 行 · 1.2s") || strings.Contains(collapsedReasoning, "line 1") || strings.Contains(collapsedReasoning, "点击") || strings.Contains(collapsedReasoning, "Ctrl+R") {
		t.Fatalf("collapsed reasoning should render only a clickable title, got %q", collapsedReasoning)
	}
	expandedReasoning := StripANSI(ReasoningBlock("line 1\nline 2", false, "1.2s"))
	if !strings.Contains(expandedReasoning, "Thought · 2 行 · 1.2s") || !strings.Contains(expandedReasoning, "line 1") || !strings.Contains(expandedReasoning, "line 2") {
		t.Fatalf("expanded reasoning should render title and body, got %q", expandedReasoning)
	}
	for _, rendered := range []string{
		Dim("dim"),
		Key("key"),
		Status("status"),
		Interrupted("stop"),
		System("system"),
		Error("error"),
		Tool("tool"),
		Reasoning("thinking"),
		Banner("banner"),
		PickerBox(20, "box"),
		PickerTitle("title"),
		PickerSelected("selected"),
		JumpToBottomButton(),
	} {
		if strings.TrimSpace(StripANSI(rendered)) == "" {
			t.Fatalf("styled helper rendered empty output: %q", rendered)
		}
	}
}

func TestToolRuntimeRendering(t *testing.T) {
	call := StripANSI(ToolCall("", `{"command":"go test ./..."}`, ToolStatusRunning))
	if !strings.Contains(call, "tool") || !strings.Contains(call, "go test ./...") {
		t.Fatalf("ToolCall = %q", call)
	}
	if strings.Contains(call, "tool(") {
		t.Fatalf("ToolCall should separate args from name without parentheses, got %q", call)
	}
	partialCall := StripANSI(ToolCall("todo_add", `{"title":"测试 TODO`, ToolStatusRunning))
	if strings.Contains(partialCall, "测试 TODO") || strings.Contains(partialCall, "(") {
		t.Fatalf("running ToolCall should hide incomplete args, got %q", partialCall)
	}
	pendingCall := StripANSI(ToolCallWithArgsReady("todo_add", `{"title":"测试 TODO"}`, ToolStatusRunning, false))
	if !strings.Contains(pendingCall, "参数准备中...") || strings.Contains(pendingCall, "测试 TODO") {
		t.Fatalf("pending ToolCall should show stable pending text, got %q", pendingCall)
	}
	readyCall := StripANSI(ToolCallWithArgsReady("todo_add", `{"title":"测试 TODO"}`, ToolStatusRunning, true))
	if !strings.Contains(readyCall, "测试 TODO") || strings.Contains(readyCall, "参数准备中") {
		t.Fatalf("ready ToolCall should show final args, got %q", readyCall)
	}

	result := StripANSI(ToolResult("exec", "{}", "line1\n\nline2\nline3", ToolStatusDone))
	if !strings.Contains(result, "exec") || !strings.Contains(result, "line1") || !strings.Contains(result, "line3") {
		t.Fatalf("ToolResult = %q", result)
	}
	if got := StripANSI(ToolResult("exec", "{}", "   ", ToolStatusDone)); !strings.Contains(got, "exec") {
		t.Fatalf("empty ToolResult should fall back to call, got %q", got)
	}
	formattedResult := StripANSI(ToolResult(
		"file_list",
		`{"path":"/tmp/project"}`,
		`{"content":"目录 /tmp/project 下的文件和文件夹:\n\n[FILE] index.ts (509 bytes)\n[DIR] interactive\n[DIR] rpc\n[FILE] readme.md (1 KB)\n[FILE] package.json (2 KB)"}`,
		ToolStatusDone,
	))
	if strings.Contains(formattedResult, `{"content"`) || strings.Contains(formattedResult, `\n`) {
		t.Fatalf("structured tool result should not render raw JSON: %q", formattedResult)
	}
	for _, want := range []string{"目录 /tmp/project", "[FILE] index.ts", "[DIR] interactive", "隐藏 2 项"} {
		if !strings.Contains(formattedResult, want) {
			t.Fatalf("structured tool result missing %q: %q", want, formattedResult)
		}
	}
	arrayResult := StripANSI(ToolResult("search", `{}`, `{"results":[{"title":"A"},{"title":"B"},{"title":"C"},{"title":"D"},{"title":"E"}]}`, ToolStatusDone))
	if !strings.Contains(arrayResult, "隐藏 1 项") || !strings.Contains(arrayResult, "- {\"title\":\"A\"}") {
		t.Fatalf("array tool result should limit preview items: %q", arrayResult)
	}

	items := []ToolChainItem{
		{Name: "old1"},
		{Name: "old2"},
		{Name: "old3"},
		{Name: "old4"},
		{Name: "old5"},
		{Name: "old6"},
		{Name: "fail", Args: `{"path":"/tmp/a"}`, Status: "error", Error: "boom happened"},
		{Name: "pending", Args: `{"path":"/tmp/draft"}`, ArgsPending: true},
	}
	lines := RenderToolChainLines(items, 40)
	joined := StripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "省略 2") || !strings.Contains(joined, "fail") || !strings.Contains(joined, "boom happened") || !strings.Contains(joined, "参数准备中...") {
		t.Fatalf("tool chain lines = %q", joined)
	}
	if got := RenderToolChainLines(nil, 40); len(got) != 1 || !strings.Contains(got[0], "等待工具") {
		t.Fatalf("empty tool chain = %#v", got)
	}

	if !isToolChainErrorStatus("失败") || !isToolChainErrorStatus("failed") || isToolChainErrorStatus("done") {
		t.Fatal("tool chain error status detection failed")
	}
	if toolStatusColor(ToolStatusRunning) == toolStatusColor(ToolStatusDone) {
		t.Fatal("running and done colors should differ")
	}
	line, hidden := toolResultPreviewLine(" a \r\n\r\n b \n c ")
	if line != "a  b" || hidden != 1 {
		t.Fatalf("toolResultPreviewLine = %q,%d", line, hidden)
	}
	if formatInt(42) != "42" {
		t.Fatal("formatInt returned unexpected value")
	}
}

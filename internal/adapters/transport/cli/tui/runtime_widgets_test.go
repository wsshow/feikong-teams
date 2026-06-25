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

	result := StripANSI(ToolResult("exec", "{}", "line1\n\nline2\nline3", ToolStatusDone))
	if !strings.Contains(result, "exec") || !strings.Contains(result, "line1") || !strings.Contains(result, "隐藏 1 行") {
		t.Fatalf("ToolResult = %q", result)
	}
	if got := StripANSI(ToolResult("exec", "{}", "   ", ToolStatusDone)); !strings.Contains(got, "exec") {
		t.Fatalf("empty ToolResult should fall back to call, got %q", got)
	}

	items := []ToolChainItem{
		{Name: "old1"},
		{Name: "old2"},
		{Name: "old3"},
		{Name: "old4"},
		{Name: "old5"},
		{Name: "old6"},
		{Name: "fail", Args: `{"path":"/tmp/a"}`, Status: "error", Error: "boom happened"},
	}
	lines := RenderToolChainLines(items, 40)
	joined := StripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "省略 1") || !strings.Contains(joined, "fail") || !strings.Contains(joined, "boom happened") {
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

package tui

import (
	"strings"
	"testing"
)

func TestInlinePasteInsertExpandAndDelete(t *testing.T) {
	value, cursor, pastes := InsertInlinePaste("hello world", 5, nil, "a\r\nb\nc")
	if value != "hello [粘贴3行内容]  world" {
		t.Fatalf("value after paste = %q", value)
	}
	if cursor != len([]rune("hello [粘贴3行内容] ")) {
		t.Fatalf("cursor after paste = %d", cursor)
	}
	if len(pastes) != 1 || pastes[0] != "a\nb\nc" {
		t.Fatalf("pastes = %#v", pastes)
	}
	expanded := ExpandInlineInput(value+InlineLineBreakTag, pastes)
	if expanded != "hello a\nb\nc  world\n" {
		t.Fatalf("expanded value = %q", expanded)
	}

	value, cursor, pastes, ok := DeleteInlinePasteBeforeCursor(value, cursor, pastes)
	if !ok {
		t.Fatal("expected paste placeholder to be deleted")
	}
	if value != "hello world" || cursor != len([]rune("hello")) || len(pastes) != 0 {
		t.Fatalf("delete paste result value=%q cursor=%d pastes=%#v", value, cursor, pastes)
	}

	value, cursor, pastes, ok = DeleteInlinePasteBeforeCursor("plain", 5, pastes)
	if ok || value != "plain" || cursor != 5 {
		t.Fatalf("unexpected delete without paste value=%q cursor=%d ok=%v", value, cursor, ok)
	}
}

func TestInlineTokenDeleteAndRender(t *testing.T) {
	value, cursor, ok := DeleteInlineTokenNearCursor("ask @coder #file.txt now", len("ask @coder"))
	if !ok {
		t.Fatal("expected mention token to be deleted")
	}
	if value != "ask #file.txt now" || cursor != len("ask") {
		t.Fatalf("delete mention result value=%q cursor=%d", value, cursor)
	}

	value, cursor, ok = DeleteInlineTokenNearCursor("ask #file.txt now", len("ask #file.txt"))
	if !ok {
		t.Fatal("expected file token to be deleted")
	}
	if value != "ask now" || cursor != len("ask") {
		t.Fatalf("delete file result value=%q cursor=%d", value, cursor)
	}

	value, cursor, ok = DeleteInlineTokenNearCursor("ask coder", len("ask coder"))
	if ok || value != "ask coder" || cursor != len("ask coder") {
		t.Fatalf("unexpected delete result value=%q cursor=%d ok=%v", value, cursor, ok)
	}

	rendered := RenderInlineInputValue("@coder #file [粘贴2行内容] " + InlineLineBreakTag)
	plain := StripANSI(rendered)
	if !strings.Contains(plain, "@coder") || !strings.Contains(plain, "#file") || !strings.Contains(plain, "[粘贴2行内容]") {
		t.Fatalf("rendered inline value lost tokens: %q", plain)
	}
	if !strings.Contains(plain, "\n  ") {
		t.Fatalf("rendered line break = %q", plain)
	}

	withCursor := RenderInlineInputValueAtCursor("abc", 1)
	if StripANSI(withCursor) != "abc" {
		t.Fatalf("cursor render visible text = %q", StripANSI(withCursor))
	}
	if endCursor := RenderInlineInputValueAtCursor("abc", 99); StripANSI(endCursor) != "abc " {
		t.Fatalf("end cursor visible text = %q", StripANSI(endCursor))
	}
}

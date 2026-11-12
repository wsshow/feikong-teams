package message

import "testing"

func TestOutputLayoutPreservesAlternatingUnicodeSegmentsAndFinalRewrite(t *testing.T) {
	var layout OutputLayout
	first := layout.Append("m", "reasoning", "先思考", "")
	text := layout.Append("m", "text", "草稿", "")
	last := layout.Append("m", "reasoning", "再思考", "")
	msg := Message{Content: "最终答案", ReasoningContent: "先思考再思考"}
	parts := layout.Complete("m", msg)
	if len(parts) != 3 || parts[0].ID != first.ID || parts[1].ID != text.ID || parts[2].ID != last.ID || msg.Content[parts[1].Start:parts[1].End] != "最终答案" {
		t.Fatalf("parts=%#v", parts)
	}
}
func TestOutputLayoutCopiesCanContinueBufferedMessages(t *testing.T) {
	var layout OutputLayout
	layout.Append("m", "text", "one", "")
	pending := layout
	pending.Append("m", "text", "two", "")
	if pending.Text != "onetwo" {
		t.Fatalf("text=%s", pending.Text)
	}
}

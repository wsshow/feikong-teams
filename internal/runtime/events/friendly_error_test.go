package events

import (
	"strings"
	"testing"
)

func TestNormalizeFriendlyErrorForUnsupportedImage(t *testing.T) {
	raw := "[coordinator] [NodeRunError] failed to generate stream request: user input multi content: deepseek does not support image_url type"

	got := NormalizeFriendlyError(raw)

	if got.Code != "model_unsupported_image_input" {
		t.Fatalf("code = %q, want model_unsupported_image_input", got.Code)
	}
	if !strings.Contains(got.Title, "不支持图片输入") {
		t.Fatalf("title = %q, want image unsupported title", got.Title)
	}
	if !strings.Contains(got.Message, "deepseek") {
		t.Fatalf("message = %q, want provider name", got.Message)
	}
	if got.TechnicalDetail != raw {
		t.Fatalf("technical detail = %q, want raw error", got.TechnicalDetail)
	}
	if len(got.Suggestions) == 0 {
		t.Fatal("expected suggestions")
	}
}

func TestNormalizeFriendlyErrorForUnknownError(t *testing.T) {
	got := NormalizeFriendlyError("unexpected boom")

	if got.Code != "unknown_error" {
		t.Fatalf("code = %q, want unknown_error", got.Code)
	}
	if got.Message == "" || got.TechnicalDetail != "unexpected boom" {
		t.Fatalf("friendly = %#v, want generic message with technical detail", got)
	}
}

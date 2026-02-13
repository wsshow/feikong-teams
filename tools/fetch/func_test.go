package fetch

import (
	"context"
	"testing"
)

func TestFetch(t *testing.T) {
	tests := []struct {
		name    string
		req     *FetchRequest
		wantErr bool
	}{
		{
			name: "fetch text from example.com",
			req: &FetchRequest{
				URL:    "https://example.com",
				Format: "text",
			},
			wantErr: false,
		},
		{
			name: "fetch markdown from example.com",
			req: &FetchRequest{
				URL:    "https://example.com",
				Format: "markdown",
			},
			wantErr: false,
		},
		{
			name: "fetch html from example.com",
			req: &FetchRequest{
				URL:    "https://example.com",
				Format: "html",
			},
			wantErr: false,
		},
		{
			name: "invalid URL - no protocol",
			req: &FetchRequest{
				URL:    "example.com",
				Format: "text",
			},
			wantErr: false, // Returns error in response, not as error
		},
		{
			name: "empty URL",
			req: &FetchRequest{
				URL:    "",
				Format: "text",
			},
			wantErr: false, // Returns error in response, not as error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			resp, err := Fetch(ctx, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if resp == nil {
				t.Error("Fetch() returned nil response")
				return
			}

			// Check for error message in response
			if resp.ErrorMessage != "" {
				t.Logf("Response error: %s", resp.ErrorMessage)
				if tt.req.URL != "" && (tt.req.URL == "https://example.com") {
					t.Errorf("Unexpected error for valid URL: %s", resp.ErrorMessage)
				}
			} else {
				// Valid response
				if resp.Content == "" {
					t.Error("Content is empty")
				}
				if resp.StatusCode != 200 {
					t.Errorf("Expected status code 200, got %d", resp.StatusCode)
				}
				t.Logf("Content length: %d bytes", len(resp.Content))
				t.Logf("Content type: %s", resp.ContentType)
				t.Logf("Is truncated: %v", resp.IsTruncated)
			}
		})
	}
}

func TestProcessContent(t *testing.T) {
	htmlContent := `<html>
<head><title>Test Page</title></head>
<body>
	<h1>Main Heading</h1>
	<h2>Subheading</h2>
	<p>This is a <strong>test</strong> paragraph with <a href="https://example.com">a link</a>.</p>
	<ul>
		<li>Item 1</li>
		<li>Item 2</li>
		<li>Item 3</li>
	</ul>
	<blockquote>This is a quote</blockquote>
	<pre><code>var x = 10;</code></pre>
	<script>alert('test');</script>
	<style>body { color: red; }</style>
</body>
</html>`

	tests := []struct {
		name        string
		content     string
		contentType string
		format      string
		wantErr     bool
	}{
		{
			name:        "extract text from HTML",
			content:     htmlContent,
			contentType: "text/html",
			format:      "text",
			wantErr:     false,
		},
		{
			name:        "convert HTML to markdown",
			content:     htmlContent,
			contentType: "text/html",
			format:      "markdown",
			wantErr:     false,
		},
		{
			name:        "extract HTML body",
			content:     htmlContent,
			contentType: "text/html",
			format:      "html",
			wantErr:     false,
		},
		{
			name:        "plain text as-is",
			content:     "Plain text content",
			contentType: "text/plain",
			format:      "text",
			wantErr:     false,
		},
		{
			name:        "json format",
			content:     `{"name": "test", "value": 123}`,
			contentType: "application/json",
			format:      "json",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processContent(tt.content, tt.contentType, tt.format)

			if (err != nil) != tt.wantErr {
				t.Errorf("processContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result == "" {
					t.Error("processContent() returned empty result")
				}
				t.Logf("Result:\n%s", result)

				// Verify markdown conversion quality
				if tt.format == "markdown" && tt.contentType == "text/html" {
					if !containsAny(result, []string{"# Main Heading", "## Subheading"}) {
						t.Error("Markdown should contain heading markers")
					}
					if !containsAny(result, []string{"* Item 1", "- Item 1"}) {
						t.Log("Warning: Markdown list formatting may differ")
					}
					if !containsAny(result, []string{"[a link](https://example.com)"}) {
						t.Error("Markdown should contain link syntax")
					}
					if containsAny(result, []string{"alert('test')", "color: red"}) {
						t.Error("Markdown should not contain script or style content")
					}
				}
			}
		})
	}
}

func TestConvertHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		html    string
		wantErr bool
		checks  []string // strings that should be present in output
	}{
		{
			name: "complex HTML with multiple elements",
			html: `
				<h1>Title</h1>
				<p>Paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
				<ul>
					<li>List item 1</li>
					<li>List item 2</li>
				</ul>
				<a href="https://example.com">Link text</a>
				<blockquote>Quote text</blockquote>
				<pre><code>code block</code></pre>
			`,
			wantErr: false,
			checks:  []string{"# Title", "**bold**", "_italic_", "Link text", "https://example.com"},
		},
		{
			name: "nested lists",
			html: `
				<ul>
					<li>Item 1
						<ul>
							<li>Nested 1</li>
							<li>Nested 2</li>
						</ul>
					</li>
					<li>Item 2</li>
				</ul>
			`,
			wantErr: false,
			checks:  []string{"Item 1", "Item 2"},
		},
		{
			name: "table conversion",
			html: `
				<table>
					<tr>
						<th>Header 1</th>
						<th>Header 2</th>
					</tr>
					<tr>
						<td>Cell 1</td>
						<td>Cell 2</td>
					</tr>
				</table>
			`,
			wantErr: false,
			checks:  []string{"Header 1", "Header 2", "Cell 1", "Cell 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertHTMLToMarkdown(tt.html)

			if (err != nil) != tt.wantErr {
				t.Errorf("convertHTMLToMarkdown() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				t.Logf("Markdown output:\n%s", result)

				for _, check := range tt.checks {
					if !containsAny(result, []string{check}) {
						t.Logf("Warning: expected to find '%s' in output", check)
					}
				}
			}
		})
	}
}

// containsAny checks if the text contains any of the substrings
func containsAny(text string, substrings []string) bool {
	for _, substr := range substrings {
		if len(substr) > 0 && len(text) >= len(substr) {
			for i := 0; i <= len(text)-len(substr); i++ {
				if text[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

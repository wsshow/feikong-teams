package memory

import (
	"testing"
)

func TestBM25BigramSearch(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Summary: "用户喜欢简洁回答", Detail: "不喜欢啰嗦的输出", Tags: []string{"偏好", "风格"}},
		{ID: "2", Summary: "用户是 Go 后端工程师", Detail: "十年 Go 开发经验", Tags: []string{"身份", "技术"}},
		{ID: "3", Summary: "避免在测试中 mock 数据库", Detail: "上次 mock 导致线上事故", Tags: []string{"教训", "测试"}},
		{ID: "4", Summary: "项目使用 Eino 框架", Detail: "确认使用 CloudWeGo Eino", Tags: []string{"项目", "框架"}},
	}

	b := &BM25{}
	b.Build(entries)

	tests := []struct {
		query string
		want  string // expected top result Summary
	}{
		{"简洁", "用户喜欢简洁回答"},
		{"Go 工程师", "用户是 Go 后端工程师"},
		{"mock 测试", "避免在测试中 mock 数据库"},
		{"Eino 框架", "项目使用 Eino 框架"},
		{"啰嗦", "用户喜欢简洁回答"}, // bigram: 啰嗦 matches "啰嗦"
		{"数据库 测试", "避免在测试中 mock 数据库"},
	}

	for _, tt := range tests {
		results := b.Search(tt.query, entries, 1)
		if len(results) == 0 {
			t.Errorf("Search(%q) returned no results", tt.query)
			continue
		}
		if results[0].Entry.Summary != tt.want {
			t.Errorf("Search(%q) top = %q, want %q", tt.query, results[0].Entry.Summary, tt.want)
		}
	}
}

func TestBigramTokenization(t *testing.T) {
	tests := []struct {
		input    string
		contains []string
		missing  []string
	}{
		{
			"喜欢简洁回答",
			[]string{"喜欢", "欢简", "简洁", "洁回", "回答"},
			[]string{"喜", "欢", "简", "洁", "回", "答"}, // no single-char tokens when bigrams available
		},
		{
			"不喜欢啰嗦的输出",
			[]string{"不喜", "喜欢", "欢啰", "啰嗦", "嗦的", "的输", "输出"},
			nil,
		},
		{
			"hello world 中文",
			[]string{"hello", "world", "中文"},
			nil,
		},
		{
			"单个汉",
			[]string{"单个", "个汉"},
			nil,
		},
		{
			"单",
			[]string{"单"},
			nil,
		},
	}

	for _, tt := range tests {
		tokens := textTokenize(tt.input)
		tokenSet := make(map[string]bool)
		for _, tok := range tokens {
			tokenSet[tok] = true
		}
		for _, want := range tt.contains {
			if !tokenSet[want] {
				t.Errorf("textTokenize(%q) missing %q, got: %v", tt.input, want, tokens)
			}
		}
		for _, avoid := range tt.missing {
			if tokenSet[avoid] {
				t.Errorf("textTokenize(%q) should NOT contain %q, got: %v", tt.input, avoid, tokens)
			}
		}
	}
}

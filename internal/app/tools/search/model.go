package search

import (
	"context"
	"net/http"
)

var (
	searchHTMLURL = "https://html.duckduckgo.com/html/"

	defaultTextSearchToolName = "duckduckgo_text_search"
	defaultTextSearchToolDesc = `DuckDuckGo plain-text web search.

Use when the answer may depend on current or external information, or when you need candidate URLs before fetching source pages.

Guidelines:
- For recent information, use time_range when appropriate and include concrete dates in the query.
- Prefer official, primary, authoritative, or original sources when available.
- Search results are candidates, not final evidence; use fetch for pages that matter or when snippets conflict.
- Avoid repeated identical searches; vary language, entity names, or source type when broadening coverage.
- When answering from web results, preserve the URLs you actually used so the final answer can cite sources.`
)

type Search interface {
	TextSearch(ctx context.Context, req *TextSearchRequest) (*TextSearchResponse, error)
}

// client 是 DuckDuckGo 搜索客户端。
type client struct {
	httpCli    *http.Client
	maxResults int
	region     Region
}

// Region 表示搜索结果地区。
type Region string

const (
	// RegionWT 表示全球地区。
	RegionWT Region = "wt-wt"
	// RegionUS 表示美国地区。
	RegionUS Region = "us-en"
	// RegionUK 表示英国地区。
	RegionUK Region = "uk-en"
	// RegionDE 表示德国地区。
	RegionDE Region = "de-de"
	// RegionFR 表示法国地区。
	RegionFR Region = "fr-fr"
	// RegionJP 表示日本地区。
	RegionJP Region = "jp-jp"
	// RegionCN 表示中国地区。
	RegionCN Region = "cn-zh"
	// RegionRU 表示俄罗斯地区。
	RegionRU Region = "ru-ru"
)

// TimeRange 表示搜索时间范围。
type TimeRange string

const (
	// TimeRangeDay 限制为过去一天。
	TimeRangeDay TimeRange = "d"
	// TimeRangeWeek 限制为过去一周。
	TimeRangeWeek TimeRange = "w"
	// TimeRangeMonth 限制为过去一月。
	TimeRangeMonth TimeRange = "m"
	// TimeRangeYear 限制为过去一年。
	TimeRangeYear TimeRange = "y"
	// TimeRangeAny 不限制时间。
	TimeRangeAny TimeRange = ""
)

type TextSearchRequest struct {
	// Query 是用户搜索词。
	Query string `json:"query" jsonschema:"description=The user's search query. The query is required.,required"`
	// TimeRange 是搜索时间范围，默认不限制。
	TimeRange TimeRange `json:"time_range,omitempty" jsonschema:"description=The time range of search results: d for past day; w for past week; m for past month; y for past year; empty for any time"`
}

// TextSearchResult 表示单条搜索结果。
type TextSearchResult struct {
	// Title 是结果标题。
	Title string `json:"title"`
	// URL 是结果地址。
	URL string `json:"url"`
	// Summary 是结果摘要。
	Summary string `json:"summary"`
}

// TextSearchResponse 表示搜索响应。
type TextSearchResponse struct {
	// Message 是给模型的简短状态信息。
	Message string `json:"message,omitempty"`
	// Results 是搜索结果列表。
	Results []*TextSearchResult `json:"results,omitempty"`
	// ErrorMessage 是给模型的错误提示。
	ErrorMessage string `json:"error_message,omitempty"`
}

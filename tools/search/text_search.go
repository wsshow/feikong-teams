package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/corpix/uarand"
)

func (c *client) TextSearch(ctx context.Context, input *TextSearchRequest) (*TextSearchResponse, error) {
	// 验证输入
	if input.Query == "" {
		return &TextSearchResponse{
			ErrorMessage: "search query is required, please provide a query string",
		}, nil
	}

	// 验证查询长度
	if len(input.Query) > 500 {
		return &TextSearchResponse{
			ErrorMessage: "search query is too long (max 500 characters), please shorten your query",
		}, nil
	}

	results := make([]*TextSearchResult, 0, c.maxResults)

	header := buildTextHTMLRequestHeader()
	reqBody := input.buildTextHTMLRequestBody(c.region)

	for {
		var req *http.Request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchHTMLURL, strings.NewReader(reqBody.Encode()))
		if err != nil {
			return &TextSearchResponse{
				ErrorMessage: fmt.Sprintf("failed to create search request: %v", err),
			}, nil
		}

		req.Header = header

		resultsTmp, nextReqBody, err := c.doTextHTMLSearch(ctx, req)
		if err != nil {
			return &TextSearchResponse{
				ErrorMessage: fmt.Sprintf("search request failed: %v. Please try again or rephrase your query", err),
			}, nil
		}

		if len(resultsTmp) == 0 {
			break
		}

		results = append(results, resultsTmp...)
		reqBody = nextReqBody

		if len(results) >= c.maxResults {
			results = results[:c.maxResults]
			break
		}

		if len(reqBody) == 0 {
			break
		}

		<-time.After(3 * time.Second) // request too fast may cause 202
	}

	if len(results) == 0 {
		return &TextSearchResponse{
			Message: "No results found for your query. Try using different keywords or broader search terms.",
		}, nil
	}

	resp := &TextSearchResponse{
		Message: fmt.Sprintf("Found %d results successfully.", len(results)),
		Results: results,
	}

	return resp, nil
}

func buildTextHTMLRequestHeader() http.Header {
	return http.Header{
		"Referer":        {"https://html.duckduckgo.com/"},
		"Sec-Fetch-Site": {"same-origin"},
		"Sec-Fetch-Dest": {"document"},
		"Sec-Fetch-Mode": {"navigate"},
		"Sec-Fetch-User": {"?1"},
		"Content-Type":   {"application/x-www-form-urlencoded"},
		"User-Agent":     {uarand.GetRandom()},
	}
}

func (t *TextSearchRequest) buildTextHTMLRequestBody(region Region) url.Values {
	// q (str): Search query string
	// s (int): Search offset for pagination
	// nextParams (str): Continuation parameters from previous page response, typically empty
	// v (str): Typically 'l' for subsequent pages
	// o (str): Output format, typically 'json'
	// dc (int): Display count - value equal to offset (s) + 1
	// api (str): API endpoint identifier, typically 'd.js'
	// vqd (str): Validation query digest
	// kl (str): Keyboard language/region code (e.g., 'en-us')
	// df (str): Time filter, maps to values like 'd' (day), 'w' (week), 'm' (month), 'y' (year)

	body := url.Values{
		"q":  {t.Query},
		"b":  {""},
		"kl": {""},
		"df": {string(TimeRangeAny)},
	}

	if region != RegionWT {
		body["kl"] = []string{string(region)}
	}

	switch t.TimeRange {
	case TimeRangeDay, TimeRangeWeek, TimeRangeMonth, TimeRangeYear:
		body["df"] = []string{string(t.TimeRange)}
	}

	return body
}

func (c *client) doTextHTMLSearch(_ context.Context, req *http.Request) (results []*TextSearchResult, nextReqBody url.Values, err error) {
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("network error, please check your connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, nil, fmt.Errorf("rate limit exceeded (status 429), please wait a moment and try again")
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, nil, fmt.Errorf("access forbidden (status 403), the search service may be blocking requests")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("search service returned status %d, please try again later", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read search results: %w", err)
	}

	results, nextReqBody, err = parseTextHTMLSearchResponse(string(respBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return
}

func parseTextHTMLSearchResponse(respBody string) (results []*TextSearchResult, nextReqBody url.Values, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(respBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var elements []*goquery.Selection
	doc.Find("table").Last().Find("tr").Each(func(i int, s *goquery.Selection) {
		elements = append(elements, s)
	})

	hrefCache := make(map[string]bool)
	results = make([]*TextSearchResult, 0, len(elements))

	doc.Find("div#links div.web-result").Each(func(i int, s *goquery.Selection) {
		title := s.Find("h2.result__title > a").First()
		if title.Length() == 0 {
			return
		}

		href, _ := title.Attr("href")
		if href == "" {
			return
		}

		if _, ok := hrefCache[href]; ok ||
			strings.HasPrefix(href, "http://www.google.com/search?q=") ||
			strings.HasPrefix(href, "https://duckduckgo.com/y.js?ad_domain") {
			return
		}

		summary := s.Find("a.result__snippet").First()
		if summary.Length() == 0 {
			return
		}

		hrefCache[href] = true

		results = append(results, &TextSearchResult{
			Title:   strings.TrimSpace(title.Text()),
			URL:     href,
			Summary: strings.TrimSpace(summary.Text()),
		})
	})

	navLinks := doc.Find("div.nav-link")
	if navLinks.Length() == 0 {
		return results, nil, nil
	}

	nextReqBody = url.Values{}

	lastForm := doc.Find("form").Last()
	lastForm.Find("input[type=hidden]").Each(func(_ int, s *goquery.Selection) {
		name, nameExist := s.Attr("name")
		value, valueExist := s.Attr("value")
		if nameExist && valueExist {
			nextReqBody.Set(name, value)
		}
	})

	return results, nextReqBody, nil
}

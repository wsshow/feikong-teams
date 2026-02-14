package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTextSearchErrorHandling(t *testing.T) {
	tests := []struct {
		name             string
		request          *TextSearchRequest
		wantErrorMessage bool
		errorContains    string
		expectResults    bool
	}{
		{
			name: "empty query",
			request: &TextSearchRequest{
				Query: "",
			},
			wantErrorMessage: true,
			errorContains:    "query is required",
			expectResults:    false,
		},
		{
			name: "query too long",
			request: &TextSearchRequest{
				Query: strings.Repeat("a", 501),
			},
			wantErrorMessage: true,
			errorContains:    "too long",
			expectResults:    false,
		},
		{
			name: "valid query",
			request: &TextSearchRequest{
				Query: "golang",
			},
			wantErrorMessage: false,
			expectResults:    false, // May not have results in test
		},
		{
			name: "valid query with time range",
			request: &TextSearchRequest{
				Query:     "golang programming",
				TimeRange: TimeRangeWeek,
			},
			wantErrorMessage: false,
			expectResults:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test client with minimal configuration
			client := &client{
				httpCli: &http.Client{
					Timeout: 10 * time.Second,
				},
				maxResults: 5,
				region:     RegionWT,
			}

			ctx := context.Background()
			resp, err := client.TextSearch(ctx, tt.request)

			// Should never return error, always return response
			if err != nil {
				t.Errorf("TextSearch() should not return error, got: %v", err)
				return
			}

			if resp == nil {
				t.Error("TextSearch() returned nil response")
				return
			}

			// Check error message
			hasErrorMessage := resp.ErrorMessage != ""
			if hasErrorMessage != tt.wantErrorMessage {
				t.Errorf("TextSearch() error message presence = %v, want %v. ErrorMessage: %s",
					hasErrorMessage, tt.wantErrorMessage, resp.ErrorMessage)
			}

			if tt.wantErrorMessage && !strings.Contains(resp.ErrorMessage, tt.errorContains) {
				t.Errorf("TextSearch() error message = %q, want to contain %q",
					resp.ErrorMessage, tt.errorContains)
			}

			// Log the response
			if resp.ErrorMessage != "" {
				t.Logf("Error message: %s", resp.ErrorMessage)
			}
			if resp.Message != "" {
				t.Logf("Status message: %s", resp.Message)
			}
			if len(resp.Results) > 0 {
				t.Logf("Found %d results", len(resp.Results))
			}
		})
	}
}

func TestTextSearchResponseStructure(t *testing.T) {
	// Test that response structure is correct
	tests := []struct {
		name     string
		response *TextSearchResponse
		valid    bool
	}{
		{
			name: "error response",
			response: &TextSearchResponse{
				ErrorMessage: "test error",
			},
			valid: true,
		},
		{
			name: "success response with results",
			response: &TextSearchResponse{
				Message: "Found 2 results",
				Results: []*TextSearchResult{
					{Title: "Test 1", URL: "https://example.com/1", Summary: "Summary 1"},
					{Title: "Test 2", URL: "https://example.com/2", Summary: "Summary 2"},
				},
			},
			valid: true,
		},
		{
			name: "no results response",
			response: &TextSearchResponse{
				Message: "No results found",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify response has expected fields
			if tt.response.ErrorMessage != "" {
				t.Logf("Has error message: %s", tt.response.ErrorMessage)
				if len(tt.response.Results) > 0 {
					t.Error("Response with error should not have results")
				}
			}

			if tt.response.Message != "" {
				t.Logf("Has status message: %s", tt.response.Message)
			}

			if len(tt.response.Results) > 0 {
				t.Logf("Has %d results", len(tt.response.Results))
				for i, result := range tt.response.Results {
					if result.Title == "" || result.URL == "" {
						t.Errorf("Result %d missing required fields", i)
					}
				}
			}
		})
	}
}

func TestDoTextHTMLSearchErrorCodes(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		errorContains string
	}{
		{
			name:          "rate limit",
			statusCode:    http.StatusTooManyRequests,
			errorContains: "rate limit",
		},
		{
			name:          "forbidden",
			statusCode:    http.StatusForbidden,
			errorContains: "forbidden",
		},
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			errorContains: "status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := &client{
				httpCli: &http.Client{
					Timeout: 5 * time.Second,
				},
				maxResults: 5,
				region:     RegionWT,
			}

			req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("test"))
			_, _, err := client.doTextHTMLSearch(context.Background(), req)

			if err == nil {
				t.Error("Expected error but got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("Error = %q, want to contain %q", err.Error(), tt.errorContains)
			}

			t.Logf("Got expected error: %v", err)
		})
	}
}

func TestBuildTextHTMLRequestBody(t *testing.T) {
	tests := []struct {
		name      string
		request   *TextSearchRequest
		region    Region
		wantQuery string
	}{
		{
			name: "basic query",
			request: &TextSearchRequest{
				Query: "test query",
			},
			region:    RegionWT,
			wantQuery: "test query",
		},
		{
			name: "query with time range",
			request: &TextSearchRequest{
				Query:     "golang",
				TimeRange: TimeRangeWeek,
			},
			region:    RegionUS,
			wantQuery: "golang",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.request.buildTextHTMLRequestBody(tt.region)

			if body.Get("q") != tt.wantQuery {
				t.Errorf("Query = %q, want %q", body.Get("q"), tt.wantQuery)
			}

			if tt.request.TimeRange != "" {
				if body.Get("df") != string(tt.request.TimeRange) {
					t.Errorf("Time range = %q, want %q", body.Get("df"), tt.request.TimeRange)
				}
			}

			if tt.region != RegionWT {
				if body.Get("kl") != string(tt.region) {
					t.Errorf("Region = %q, want %q", body.Get("kl"), tt.region)
				}
			}

			t.Logf("Request body: %v", body)
		})
	}
}

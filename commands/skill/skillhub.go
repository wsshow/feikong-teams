package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type skillhubProvider struct {
	baseURL string
}

// NewSkillHubProvider 创建 SkillHub 后端
func NewSkillHubProvider(baseURL string) Provider {
	return &skillhubProvider{baseURL: baseURL}
}

func (p *skillhubProvider) Name() string { return "SkillHub" }

func (p *skillhubProvider) Search(ctx context.Context, keyword string, page, pageSize int, sortBy, order string) (*SearchResponse, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if sortBy == "" {
		sortBy = "downloads"
	}
	if order == "" {
		order = "desc"
	}
	q := u.Query()
	q.Set("keyword", keyword)
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("pageSize", fmt.Sprintf("%d", pageSize))
	q.Set("sortBy", sortBy)
	q.Set("order", order)
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	var apiResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Skills []struct {
				Name        string `json:"name"`
				Slug        string `json:"slug"`
				Description string `json:"description"`
				DescZh      string `json:"description_zh"`
				OwnerName   string `json:"ownerName"`
				Homepage    string `json:"homepage"`
				Version     string `json:"version"`
				Downloads   int    `json:"downloads"`
				Stars       int    `json:"stars"`
			} `json:"skills"`
			Total int `json:"total"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	result := &SearchResponse{Total: apiResp.Data.Total}
	for _, s := range apiResp.Data.Skills {
		result.Skills = append(result.Skills, SkillResult{
			Name:        s.Name,
			Slug:        s.Slug,
			Description: s.Description,
			DescZh:      s.DescZh,
			Owner:       s.OwnerName,
			Homepage:    s.Homepage,
			Version:     s.Version,
			Downloads:   s.Downloads,
			Stars:       s.Stars,
		})
	}
	return result, nil
}

func (p *skillhubProvider) Download(ctx context.Context, slug, version string) (io.ReadCloser, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = "/api/download"
	q := url.Values{}
	q.Set("slug", slug)
	if version != "" {
		q.Set("version", version)
	}
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return resp.Body, nil
}

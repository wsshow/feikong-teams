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

// SkillResult 统一的技能搜索结果
type SkillResult struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	DescZh      string `json:"description_zh,omitempty"`
	Owner       string `json:"owner"`
	Homepage    string `json:"homepage"`
	Version     string `json:"version"`
	Downloads   int    `json:"downloads"`
	Stars       int    `json:"stars"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Skills []SkillResult `json:"skills"`
	Total  int           `json:"total"`
}

// Provider 技能搜索后端接口
type Provider interface {
	Name() string
	Search(ctx context.Context, keyword string, page, pageSize int) (*SearchResponse, error)
}

// --- LightMake 后端实现 ---

type lightmakeProvider struct {
	baseURL string
}

// NewLightMakeProvider 创建 LightMake 搜索后端
func NewLightMakeProvider(baseURL string) Provider {
	return &lightmakeProvider{baseURL: baseURL}
}

func (p *lightmakeProvider) Name() string { return "lightmake" }

func (p *lightmakeProvider) Search(ctx context.Context, keyword string, page, pageSize int) (*SearchResponse, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	q := u.Query()
	q.Set("keyword", keyword)
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("pageSize", fmt.Sprintf("%d", pageSize))
	q.Set("sortBy", "downloads")
	q.Set("order", "desc")
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

// --- 默认 Provider 注册 ---

var defaultProviders = []Provider{
	NewLightMakeProvider("https://lightmake.site/api/skills"),
}

// GetProviders 返回所有注册的搜索后端
func GetProviders() []Provider {
	return defaultProviders
}

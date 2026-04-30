package skill

import (
	"context"
	"fmt"
	"io"
	"strings"
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

// Provider 技能后端接口
type Provider interface {
	Name() string
	Search(ctx context.Context, keyword string, page, pageSize int, sortBy, order string) (*SearchResponse, error)
	Download(ctx context.Context, slug, version string) (io.ReadCloser, error)
}

// --- 默认 Provider 注册 ---

var defaultProviders = []Provider{
	NewSkillHubProvider("https://lightmake.site/api/skills"),
}

// GetProviders 返回所有注册的后端
func GetProviders() []Provider {
	return defaultProviders
}

// GetDefaultProvider 返回默认的后端
func GetDefaultProvider() Provider {
	if len(defaultProviders) > 0 {
		return defaultProviders[0]
	}
	return nil
}

// GetProviderByName 按名称查找后端（不区分大小写）
func GetProviderByName(name string) Provider {
	for _, p := range defaultProviders {
		if strings.EqualFold(p.Name(), name) {
			return p
		}
	}
	return nil
}

// GetProvidersByNames 按名称列表查找后端，返回匹配的后端列表
// 如果 names 为空，返回所有后端
func GetProvidersByNames(names []string) ([]Provider, error) {
	if len(names) == 0 {
		return defaultProviders, nil
	}
	var result []Provider
	for _, name := range names {
		p := GetProviderByName(name)
		if p == nil {
			return nil, fmt.Errorf("未找到后端: %s", name)
		}
		result = append(result, p)
	}
	return result, nil
}

// ProviderNames 返回所有后端名称
func ProviderNames() []string {
	names := make([]string, len(defaultProviders))
	for i, p := range defaultProviders {
		names[i] = p.Name()
	}
	return names
}

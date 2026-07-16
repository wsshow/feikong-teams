// Package skillhub 提供 SkillHub 技能市场 HTTP 适配器。
package skillhub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	appskill "fkteams/internal/app/skill"
)

const (
	maxSearchResponseBytes int64 = 8 << 20
	maxSkillDownloadBytes  int64 = 64 << 20
	maxErrorDrainBytes     int64 = 64 << 10
	minSearchPageSize            = 1
	maxSearchPageSize            = 100
)

type Provider struct {
	baseURL string
	client  *http.Client
}

func New(baseURL string, client *http.Client) *Provider {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &Provider{baseURL: baseURL, client: client}
}

func (p *Provider) Name() string { return "SkillHub" }

func (p *Provider) Search(ctx context.Context, keyword string, page, pageSize int, sortBy, order string) (*appskill.SearchResponse, error) {
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
	if page < 1 {
		page = 1
	}
	if pageSize < minSearchPageSize {
		pageSize = minSearchPageSize
	}
	if pageSize > maxSearchPageSize {
		pageSize = maxSearchPageSize
	}
	query := u.Query()
	query.Set("keyword", keyword)
	query.Set("page", strconv.Itoa(page))
	query.Set("pageSize", strconv.Itoa(pageSize))
	query.Set("sortBy", sortBy)
	query.Set("order", order)
	u.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("search skills: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorDrainBytes))
		return nil, fmt.Errorf("skill provider returned %d", response.StatusCode)
	}

	var payload struct {
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
	if err := decodeSearchResponse(response.Body, &payload); err != nil {
		return nil, fmt.Errorf("decode skill search response: %w", err)
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("skill provider error: %s", payload.Message)
	}

	result := &appskill.SearchResponse{Total: payload.Data.Total, Skills: make([]appskill.SkillResult, 0, len(payload.Data.Skills))}
	for _, item := range payload.Data.Skills {
		result.Skills = append(result.Skills, appskill.SkillResult{
			Name: item.Name, Slug: item.Slug, Description: item.Description, DescZh: item.DescZh,
			Owner: item.OwnerName, Homepage: item.Homepage, Version: item.Version,
			Downloads: item.Downloads, Stars: item.Stars,
		})
	}
	return result, nil
}

func (p *Provider) Download(ctx context.Context, slug, version string) (io.ReadCloser, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = "/api/download"
	query := u.Query()
	query.Set("slug", slug)
	if version != "" {
		query.Set("version", version)
	}
	u.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download skill: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorDrainBytes))
		response.Body.Close()
		return nil, fmt.Errorf("skill provider returned %d", response.StatusCode)
	}
	if response.ContentLength > maxSkillDownloadBytes {
		response.Body.Close()
		return nil, fmt.Errorf("skill archive exceeds %d bytes", maxSkillDownloadBytes)
	}
	return &limitedReadCloser{
		Reader: io.LimitReader(response.Body, maxSkillDownloadBytes+1),
		Closer: response.Body,
	}, nil
}

func decodeSearchResponse(reader io.Reader, target any) error {
	data, err := io.ReadAll(io.LimitReader(reader, maxSearchResponseBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maxSearchResponseBytes {
		return fmt.Errorf("skill search response exceeds %d bytes", maxSearchResponseBytes)
	}
	return json.Unmarshal(data, target)
}

type limitedReadCloser struct {
	io.Reader
	io.Closer
}

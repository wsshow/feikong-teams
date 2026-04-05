package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	clientID       = "Iv1.b507a08c87ecfe98" // GitHub Copilot OAuth App ID
	deviceCodeURL  = "https://github.com/login/device/code"
	accessTokenURL = "https://github.com/login/oauth/access_token"
)

// DeviceCode GitHub OAuth 设备码响应
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// RequestDeviceCode 发起 GitHub OAuth 设备码请求
func RequestDeviceCode(ctx context.Context) (*DeviceCode, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("scope", "read:user")

	req, err := http.NewRequestWithContext(ctx, "POST", deviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建设备码请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("设备码请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("设备码请求返回非 200 状态: %d", resp.StatusCode)
	}

	var dc DeviceCode
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return nil, fmt.Errorf("解析设备码响应失败: %w", err)
	}
	return &dc, nil
}

// PollForToken 轮询获取 GitHub access_token，再交换为 Copilot token
func PollForToken(ctx context.Context, dc *DeviceCode) (*Token, error) {
	interval := dc.Interval
	if interval < 5 {
		interval = 5
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("设备码授权超时")
			}

			githubToken, err := tryGetAccessToken(ctx, dc.DeviceCode)
			if err == errPending {
				continue
			}
			if err == errSlowDown {
				interval += 5
				ticker.Reset(time.Duration(interval) * time.Second)
				continue
			}
			if err != nil {
				return nil, err
			}

			// 用 GitHub token 交换 Copilot token
			token, err := exchangeCopilotToken(ctx, githubToken)
			if err != nil {
				return nil, fmt.Errorf("交换 Copilot token 失败: %w", err)
			}
			return token, nil
		}
	}
}

var (
	errPending  = fmt.Errorf("authorization_pending")
	errSlowDown = fmt.Errorf("slow_down")
)

func tryGetAccessToken(ctx context.Context, deviceCode string) (string, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, "POST", accessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	switch result.Error {
	case "":
		if result.AccessToken == "" {
			return "", errPending
		}
		return result.AccessToken, nil
	case "authorization_pending":
		return "", errPending
	case "slow_down":
		return "", errSlowDown
	default:
		return "", fmt.Errorf("OAuth 错误: %s", result.Error)
	}
}

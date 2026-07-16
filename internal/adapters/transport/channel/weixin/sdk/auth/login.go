package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fkteams/internal/adapters/transport/channel/weixin/sdk/protocol"
	"fkteams/internal/runtime/atomicfile"
)

const maxCredentialsBytes int64 = 64 << 10

// Credentials 表示机器人认证数据。
type Credentials struct {
	Token     string `json:"token"`
	BaseURL   string `json:"baseUrl"`
	AccountID string `json:"accountId"`
	UserID    string `json:"userId"`
	SavedAt   string `json:"savedAt,omitempty"`
}

// DefaultCredPath 返回默认凭证路径。
func DefaultCredPath() string {
	return filepath.Join("channels", "weixin", "credentials.json")
}

// LoadCredentials 从磁盘加载凭证。
func LoadCredentials(path string) (*Credentials, error) {
	if path == "" {
		path = DefaultCredPath()
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open credentials: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxCredentialsBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read credentials: %w", readErr)
	}
	if int64(len(data)) > maxCredentialsBytes {
		return nil, fmt.Errorf("credentials exceed %d bytes", maxCredentialsBytes)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close credentials: %w", closeErr)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("decode credentials: %w", err)
	}
	if err := validateCredentials(&creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// SaveCredentials 将凭证保存到磁盘。
func SaveCredentials(creds *Credentials, path string) error {
	if err := validateCredentials(creds); err != nil {
		return err
	}
	if path == "" {
		path = DefaultCredPath()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	data = append(data, '\n')
	if int64(len(data)) > maxCredentialsBytes {
		return fmt.Errorf("credentials exceed %d bytes", maxCredentialsBytes)
	}
	if err := atomicfile.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	return nil
}

// ClearCredentials 删除已保存凭证。
func ClearCredentials(path string) error {
	if path == "" {
		path = DefaultCredPath()
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}

func validateCredentials(creds *Credentials) error {
	if creds == nil {
		return fmt.Errorf("credentials are nil")
	}
	if strings.TrimSpace(creds.Token) == "" {
		return fmt.Errorf("credentials token is required")
	}
	if strings.TrimSpace(creds.BaseURL) == "" {
		return fmt.Errorf("credentials base URL is required")
	}
	if strings.TrimSpace(creds.AccountID) == "" {
		return fmt.Errorf("credentials account ID is required")
	}
	if strings.TrimSpace(creds.UserID) == "" {
		return fmt.Errorf("credentials user ID is required")
	}
	return nil
}

// LoginOptions 配置登录流程。
type LoginOptions struct {
	BaseURL   string
	CredPath  string
	Force     bool
	OnQRURL   func(url string)
	OnScanned func()
	OnExpired func()
}

// Login 执行扫码登录并返回凭证。
func Login(ctx context.Context, client *protocol.Client, opts LoginOptions) (*Credentials, error) {
	if client == nil {
		return nil, fmt.Errorf("protocol client is nil")
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = protocol.DefaultBaseURL
	}

	if !opts.Force {
		creds, err := LoadCredentials(opts.CredPath)
		if err != nil {
			return nil, fmt.Errorf("load credentials: %w", err)
		}
		if creds != nil {
			return creds, nil
		}
	}

	for {
		qr, err := client.GetQRCode(ctx, baseURL)
		if err != nil {
			return nil, fmt.Errorf("get QR code: %w", err)
		}

		if opts.OnQRURL != nil {
			opts.OnQRURL(qr.QRCodeImgURL)
		} else {
			fmt.Fprintf(os.Stderr, "[wechatbot] Scan this URL in WeChat: %s\n", qr.QRCodeImgURL)
		}

		lastStatus := ""
		for {
			status, err := client.PollQRStatus(ctx, baseURL, qr.QRCode)
			if err != nil {
				return nil, fmt.Errorf("poll QR status: %w", err)
			}

			if status.Status != lastStatus {
				lastStatus = status.Status
				switch status.Status {
				case "scaned":
					if opts.OnScanned != nil {
						opts.OnScanned()
					} else {
						fmt.Fprintln(os.Stderr, "[wechatbot] QR scanned — confirm in WeChat")
					}
				case "expired":
					if opts.OnExpired != nil {
						opts.OnExpired()
					} else {
						fmt.Fprintln(os.Stderr, "[wechatbot] QR expired — requesting new one")
					}
				case "confirmed":
					fmt.Fprintln(os.Stderr, "[wechatbot] Login confirmed")
				}
			}

			if status.Status == "confirmed" {
				if status.BotToken == "" || status.BotID == "" || status.UserID == "" {
					return nil, fmt.Errorf("login confirmed but missing credentials")
				}
				resolvedBase := baseURL
				if status.BaseURL != "" {
					resolvedBase = status.BaseURL
				}
				creds := &Credentials{
					Token:     status.BotToken,
					BaseURL:   resolvedBase,
					AccountID: status.BotID,
					UserID:    status.UserID,
					SavedAt:   time.Now().UTC().Format(time.RFC3339),
				}
				if err := SaveCredentials(creds, opts.CredPath); err != nil {
					fmt.Fprintf(os.Stderr, "[wechatbot] Warning: could not save credentials: %v\n", err)
				}
				return creds, nil
			}

			if status.Status == "expired" {
				break // Outer loop gets a new QR
			}

			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
}

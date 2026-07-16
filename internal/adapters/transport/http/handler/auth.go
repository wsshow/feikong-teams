package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fkteams/internal/app/config"
	"fkteams/internal/runtime/log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	authCookieName = "fk_token"
	authTokenTTL   = 7 * 24 * time.Hour
)

func getTokenSecret() []byte {
	auth := config.Get().Server.Auth
	sum := sha256.Sum256([]byte(auth.Secret + "\x00" + auth.Username + "\x00" + auth.Password))
	return sum[:]
}

// AuthEnabled 检查是否启用登录认证，启用时校验 SECRET 非空
func AuthEnabled() (bool, error) {
	auth := config.Get().Server.Auth
	if !auth.Enabled {
		return false, nil
	}
	if err := auth.Validate(); err != nil {
		return false, err
	}
	return true, nil
}

func generateToken(username string) string {
	expiry := time.Now().Add(authTokenTTL)
	payload := username + "|" + expiry.Format(time.RFC3339)
	mac := hmac.New(sha256.New, getTokenSecret())
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return hex.EncodeToString([]byte(payload)) + "." + sig
}

// ValidateToken 校验 token 有效性
func ValidateToken(token string) bool {
	parts := splitToken(token)
	if parts == nil {
		return false
	}
	payload, sig := parts[0], parts[1]

	payloadBytes, err := hex.DecodeString(payload)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, getTokenSecret())
	mac.Write(payloadBytes)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return false
	}

	// 检查过期
	payloadStr := string(payloadBytes)
	idx := strings.LastIndex(payloadStr, "|")
	if idx < 0 {
		return false
	}
	if payloadStr[:idx] != config.Get().Server.Auth.Username {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, payloadStr[idx+1:])
	if err != nil {
		return false
	}
	return time.Now().Before(expiry)
}

// RequestAuthToken 从 Bearer Header 或 HttpOnly Cookie 提取请求 Token。
// Token 不接受 query 参数，避免进入访问日志、浏览器历史和 Referer。
func RequestAuthToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return authHeader[7:]
	}
	if cookie, err := c.Cookie(authCookieName); err == nil {
		return cookie
	}
	return ""
}

func credentialsMatch(actualUsername, actualPassword, expectedUsername, expectedPassword string) bool {
	actualUserHash := sha256.Sum256([]byte(actualUsername))
	expectedUserHash := sha256.Sum256([]byte(expectedUsername))
	actualPasswordHash := sha256.Sum256([]byte(actualPassword))
	expectedPasswordHash := sha256.Sum256([]byte(expectedPassword))
	return subtle.ConstantTimeCompare(actualUserHash[:], expectedUserHash[:]) == 1 &&
		subtle.ConstantTimeCompare(actualPasswordHash[:], expectedPasswordHash[:]) == 1
}

func setAuthCookie(c *gin.Context, token string, maxAge int) {
	cookie := &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestUsesHTTPS(c.Request),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   maxAge,
	}
	if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(c.Writer, cookie)
}

func requestUsesHTTPS(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")), "https")
}

func splitToken(token string) []string {
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			return []string{token[:i], token[i+1:]}
		}
	}
	return nil
}

// LoginHandler 处理登录请求
func LoginHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		authEnabled, err := AuthEnabled()
		if err != nil {
			Fail(c, http.StatusInternalServerError, "invalid authentication configuration")
			return
		}
		if !authEnabled {
			Fail(c, http.StatusNotFound, "authentication is disabled")
			return
		}

		var req struct {
			Username   string `json:"username"`
			Password   string `json:"password"`
			CookieOnly bool   `json:"cookie_only"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "请求格式错误")
			return
		}

		auth := config.Get().Server.Auth
		expectedUser := auth.Username
		expectedPass := auth.Password

		if !credentialsMatch(req.Username, req.Password, expectedUser, expectedPass) {
			log.Printf("login failed: username=%s, ip=%s", req.Username, c.ClientIP())
			Fail(c, http.StatusUnauthorized, "用户名或密码错误")
			return
		}

		token := generateToken(req.Username)
		setAuthCookie(c, token, int(authTokenTTL/time.Second))
		c.Header("Cache-Control", "no-store")
		if req.CookieOnly {
			OK(c, gin.H{"authenticated": true})
			return
		}
		OK(c, gin.H{"token": token})
	}
}

// LogoutHandler 清除浏览器登录 Cookie。该接口允许在 Token 失效后调用。
func LogoutHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		setAuthCookie(c, "", -1)
		c.Header("Cache-Control", "no-store")
		OK(c, nil)
	}
}

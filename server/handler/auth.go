package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func getTokenSecret() []byte {
	return []byte(os.Getenv("FEIKONG_LOGIN_SECRET"))
}

// AuthEnabled 检查是否启用登录认证，启用时校验 SECRET 非空
func AuthEnabled() bool {
	if !strings.EqualFold(os.Getenv("FEIKONG_LOGIN_ENABLED"), "true") {
		return false
	}
	if os.Getenv("FEIKONG_LOGIN_SECRET") == "" {
		log.Fatalf("启用登录认证时 FEIKONG_LOGIN_SECRET 不能为空")
	}
	return true
}

func generateToken(username string) string {
	expiry := time.Now().Add(7 * 24 * time.Hour)
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
	expiry, err := time.Parse(time.RFC3339, payloadStr[idx+1:])
	if err != nil {
		return false
	}
	return time.Now().Before(expiry)
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
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "请求格式错误")
			return
		}

		expectedUser := os.Getenv("FEIKONG_LOGIN_USERNAME")
		expectedPass := os.Getenv("FEIKONG_LOGIN_PASSWORD")

		if req.Username != expectedUser || req.Password != expectedPass {
			Fail(c, http.StatusUnauthorized, "用户名或密码错误")
			return
		}

		token := generateToken(req.Username)
		OK(c, gin.H{"token": token})
	}
}

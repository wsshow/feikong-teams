package middleware

import (
	"crypto/hmac"
	"fkteams/config"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyAuth 校验 OpenAI 兼容 API 的访问密钥
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		keys := config.Get().OpenAIAPI.APIKeys
		if len(keys) == 0 {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "invalid API key",
					"type":    "invalid_api_key",
				},
			})
			return
		}

		key := []byte(authHeader[7:])
		// 使用常量时间比较防止时序攻击
		for _, k := range keys {
			if hmac.Equal(key, []byte(k)) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"message": "invalid API key",
				"type":    "invalid_api_key",
			},
		})
	}
}

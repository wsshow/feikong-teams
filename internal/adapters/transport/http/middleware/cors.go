// Package middleware 提供 HTTP 中间件（CORS、认证等）
package middleware

import (
	"net/http"

	"fkteams/internal/adapters/transport/http/origin"

	"github.com/gin-gonic/gin"
)

// Cors 返回跨域请求处理中间件
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		c.Header("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")

		allowedOrigin, ok := origin.AllowedOrigin(c.Request)
		if c.Request.Header.Get("Origin") != "" && !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		if ok {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID, X-Preview-Password")
			c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
			if allowedOrigin != "*" {
				c.Header("Access-Control-Allow-Credentials", "true")
			}
		}

		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

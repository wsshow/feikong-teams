package middleware

import (
	"fkteams/server/handler"
	"fkteams/web"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Auth 验证请求的 token，未登录时重定向到登录页
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 登录页和登录接口不需要验证
		if path == "/login" || path == "/api/fkteams/login" {
			c.Next()
			return
		}

		// 静态资源不需要验证（CSS/JS/字体等）
		if strings.HasPrefix(path, "/static/") {
			c.Next()
			return
		}

		// 从 Authorization header、query 参数或 cookie 获取 token
		token := ""
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = authHeader[7:]
		}
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			if cookie, err := c.Cookie("fk_token"); err == nil {
				token = cookie
			}
		}

		if token == "" || !handler.ValidateToken(token) {
			// API 请求返回 401
			if strings.HasPrefix(path, "/api/") || path == "/ws" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code":    1,
					"message": "未登录或登录已过期",
				})
				return
			}
			// 页面请求返回登录页
			serveLoginPage(c)
			c.Abort()
			return
		}

		c.Next()
	}
}

func serveLoginPage(c *gin.Context) {
	webFS := web.GetFS()
	data, err := webFS.Open("login.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "login page not found")
		return
	}
	defer data.Close()
	c.DataFromReader(http.StatusOK, -1, "text/html; charset=utf-8", data, nil)
}

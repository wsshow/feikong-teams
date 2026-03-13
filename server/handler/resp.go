package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一 API 响应结构
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 返回成功响应
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: data})
}

// Fail 返回失败响应
func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, Response{Code: 1, Message: msg})
}

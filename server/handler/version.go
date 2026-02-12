package handler

import (
	"fkteams/version"

	"github.com/gin-gonic/gin"
)

func VersionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		OK(c, version.Get())
	}
}

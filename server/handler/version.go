package handler

import (
	"fkteams/version"
	"net/http"

	"github.com/gin-gonic/gin"
)

func VersionHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, resp.Success(version.Get()))
	}
}

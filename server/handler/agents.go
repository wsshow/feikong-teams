package handler

import (
	"fkteams/agents"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AgentInfoResponse 智能体信息响应
type AgentInfoResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetAgentsHandler 获取所有可用智能体
func GetAgentsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		registry := agents.GetRegistry()

		// 转换为响应格式
		agentList := make([]AgentInfoResponse, 0, len(registry))
		for _, agent := range registry {
			agentList = append(agentList, AgentInfoResponse{
				Name:        agent.Name,
				Description: agent.Description,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data":    agentList,
		})
	}
}

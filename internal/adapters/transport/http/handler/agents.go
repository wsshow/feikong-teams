package handler

import (
	"fkteams/internal/app/agent/catalog"

	"github.com/gin-gonic/gin"
)

// AgentInfoResponse 智能体信息响应
type AgentInfoResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases,omitempty"`
}

// GetAgentsHandler 获取所有可用智能体
func GetAgentsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		registry := agents.GetRegistry()

		agentList := make([]AgentInfoResponse, 0, len(registry))
		for _, agent := range registry {
			agentList = append(agentList, AgentInfoResponse{
				Name:        agent.Name,
				Description: agent.Description,
				Aliases:     agent.Aliases,
			})
		}

		OK(c, agentList)
	}
}

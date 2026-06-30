package handler

import (
	"fkteams/internal/app/agent/catalog"

	"github.com/gin-gonic/gin"
)

// AgentInfoResponse 智能体信息响应
type AgentInfoResponse struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases,omitempty"`
	Builtin     bool     `json:"builtin,omitempty"`
}

// GetAgentsHandler 获取所有可用智能体
func GetAgentsHandler() gin.HandlerFunc {
	return NewRuntime().GetAgentsHandler()
}

func (rt *Runtime) GetAgentsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		registry, err := agents.List(rt.withRuntimeContext(c.Request.Context()))
		if err != nil {
			Fail(c, 500, err.Error())
			return
		}

		agentList := make([]AgentInfoResponse, 0, len(registry))
		for _, agent := range registry {
			agentList = append(agentList, AgentInfoResponse{
				Name:        agent.Name,
				DisplayName: agent.DisplayName,
				Description: agent.Description,
				Aliases:     agent.Aliases,
				Builtin:     agent.Builtin,
			})
		}

		OK(c, agentList)
	}
}

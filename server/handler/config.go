package handler

import (
	"fkteams/agents"
	"fkteams/config"
	"fkteams/tools"
	"fkteams/tools/mcp"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// maskAPIKey 对 API Key 做脱敏处理，只保留后 4 位
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(key)-4) + key[len(key)-4:]
}

const sensitivePassword = "***"

// GetConfigHandler 获取配置（敏感字段脱敏）
func GetConfigHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()

		// 深拷贝并脱敏
		resp := *cfg

		// 脱敏模型 APIKey
		models := make([]config.ModelConfig, len(cfg.Models))
		for i, m := range cfg.Models {
			models[i] = m
			models[i].APIKey = maskAPIKey(m.APIKey)
		}
		resp.Models = models

		// 脱敏 Auth
		if resp.Server.Auth.Password != "" {
			resp.Server.Auth.Password = sensitivePassword
		}
		if resp.Server.Auth.Secret != "" {
			resp.Server.Auth.Secret = sensitivePassword
		}

		// 脱敏 SSH
		if resp.Agents.SSHVisitor.Password != "" {
			resp.Agents.SSHVisitor.Password = sensitivePassword
		}

		// 脱敏 Channels
		if resp.Channels.QQ.AppSecret != "" {
			resp.Channels.QQ.AppSecret = sensitivePassword
		}
		if resp.Channels.Discord.Token != "" {
			resp.Channels.Discord.Token = maskAPIKey(resp.Channels.Discord.Token)
		}

		OK(c, resp)
	}
}

// UpdateConfigHandler 更新配置（敏感字段合并旧值）
func UpdateConfigHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var newCfg config.Config
		if err := c.ShouldBindJSON(&newCfg); err != nil {
			Fail(c, http.StatusBadRequest, "invalid config: "+err.Error())
			return
		}

		oldCfg := config.Get()

		// 合并敏感字段：前端传 "***" 或掩码时保留旧值
		for i := range newCfg.Models {
			if strings.Contains(newCfg.Models[i].APIKey, "****") {
				// 查找旧配置中同名模型的 APIKey
				for _, old := range oldCfg.Models {
					if old.Name == newCfg.Models[i].Name {
						newCfg.Models[i].APIKey = old.APIKey
						break
					}
				}
			}
		}
		if newCfg.Server.Auth.Password == sensitivePassword {
			newCfg.Server.Auth.Password = oldCfg.Server.Auth.Password
		}
		if newCfg.Server.Auth.Secret == sensitivePassword {
			newCfg.Server.Auth.Secret = oldCfg.Server.Auth.Secret
		}
		if newCfg.Agents.SSHVisitor.Password == sensitivePassword {
			newCfg.Agents.SSHVisitor.Password = oldCfg.Agents.SSHVisitor.Password
		}
		if newCfg.Channels.QQ.AppSecret == sensitivePassword {
			newCfg.Channels.QQ.AppSecret = oldCfg.Channels.QQ.AppSecret
		}
		if strings.Contains(newCfg.Channels.Discord.Token, "****") {
			newCfg.Channels.Discord.Token = oldCfg.Channels.Discord.Token
		}

		// 检测 Auth 是否变更
		authChanged := oldCfg.Server.Auth.Username != newCfg.Server.Auth.Username ||
			oldCfg.Server.Auth.Password != newCfg.Server.Auth.Password ||
			oldCfg.Server.Auth.Secret != newCfg.Server.Auth.Secret ||
			oldCfg.Server.Auth.Enabled != newCfg.Server.Auth.Enabled

		// 保存并重载
		if err := config.Save(&newCfg); err != nil {
			Fail(c, http.StatusInternalServerError, "failed to save config: "+err.Error())
			return
		}

		// 重载智能体注册表和 MCP 工具缓存
		agents.ReloadRegistry()
		mcp.ClearCache()

		OK(c, gin.H{"auth_changed": authChanged})
	}
}

// GetToolNamesHandler 获取可用工具名列表
func GetToolNamesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		OK(c, tools.GetAllToolNames())
	}
}

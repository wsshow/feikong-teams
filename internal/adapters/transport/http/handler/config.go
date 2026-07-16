package handler

import (
	"context"
	memorymodel "fkteams/internal/adapters/model/memory"
	"fkteams/internal/app/agent/catalog"
	"fkteams/internal/app/appstate"
	"fkteams/internal/app/config"
	"fkteams/internal/app/tools"
	"fkteams/internal/runtime/log"
	modelregistry "fkteams/internal/runtime/model"
	"fmt"
	"net/http"
	"runtime"
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

// isMasked 检测字段值是否为脱敏后的掩码值
func isMasked(s string) bool {
	return strings.Contains(s, "**")
}

const sensitivePassword = "***"

// GetConfigHandler 获取配置（敏感字段脱敏）
func GetConfigHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		resp := config.Snapshot()
		cfg := resp

		// 处理模型 APIKey：不返回真实密钥，仅标记是否已配置
		models := make([]config.ModelConfig, len(cfg.Models))
		for i, m := range cfg.Models {
			models[i] = m
			models[i].OriginalID = m.ID
			models[i].HasAPIKey = m.APIKey != ""
			models[i].APIKey = ""
			if m.ExtraHeaders != "" {
				models[i].ExtraHeaders = sensitivePassword
			}
		}
		resp.Models = models
		resp.OpenAIAPI.APIKeys = make([]string, len(cfg.OpenAIAPI.APIKeys))
		for i, apiKey := range cfg.OpenAIAPI.APIKeys {
			resp.OpenAIAPI.APIKeys[i] = maskAPIKey(apiKey)
		}

		// 脱敏 Auth
		if resp.Server.Auth.Password != "" {
			resp.Server.Auth.Password = sensitivePassword
		}
		if resp.Server.Auth.Secret != "" {
			resp.Server.Auth.Secret = sensitivePassword
		}

		resp.Agents.Items = agents.ConfigItems(cfg)
		maskAgentSSHPasswords(resp.Agents.Items)

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

// UpdateConfigHandlerWithState 更新配置并使用显式应用状态重载依赖。

// UpdateConfigHandlerWithState 更新配置并清理当前 HTTP runtime 的运行缓存。
func (rt *Runtime) UpdateConfigHandlerWithState(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		var newCfg config.Config
		if err := c.ShouldBindJSON(&newCfg); err != nil {
			Fail(c, http.StatusBadRequest, "invalid config: "+err.Error())
			return
		}

		oldCfg := config.Get()

		if err := newCfg.ValidateModels(); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := newCfg.ValidateRoundtable(); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if err := newCfg.ValidateDeep(); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		// 合并敏感字段：只按稳定 ID 恢复，禁止按数组位置猜测密钥归属。
		if err := restoreModelSecrets(&newCfg, oldCfg); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		apiKeys, err := restoreMaskedAPIKeys(newCfg.OpenAIAPI.APIKeys, oldCfg.OpenAIAPI.APIKeys)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		newCfg.OpenAIAPI.APIKeys = apiKeys
		if newCfg.Server.Auth.Password == sensitivePassword {
			newCfg.Server.Auth.Password = oldCfg.Server.Auth.Password
		}
		if newCfg.Server.Auth.Secret == sensitivePassword {
			newCfg.Server.Auth.Secret = oldCfg.Server.Auth.Secret
		}
		newCfg.Agents.Items = userAgentConfigItems(newCfg.Agents.Items)
		restoreAgentSSHPasswords(newCfg.Agents.Items, oldCfg)
		if newCfg.Channels.QQ.AppSecret == sensitivePassword {
			newCfg.Channels.QQ.AppSecret = oldCfg.Channels.QQ.AppSecret
		}
		if isMasked(newCfg.Channels.Discord.Token) {
			newCfg.Channels.Discord.Token = oldCfg.Channels.Discord.Token
		}
		if err := newCfg.Server.Validate(); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
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

		// 重载智能体注册表、清除 Runner 缓存和 MCP 工具缓存
		if rt.AgentRegistry != nil {
			rt.AgentRegistry.Reload()
		}
		rt.clearRunnerCache()
		if rt.ToolRegistry != nil {
			rt.ToolRegistry.ClearMCPToolCache()
		}
		if rt.ResetChannels != nil {
			rt.ResetChannels()
		}
		resetMemoryLLM(c.Request.Context(), state, rt.ModelRegistry)

		OK(c, gin.H{"auth_changed": authChanged})
	}
}

func restoreModelSecrets(newCfg *config.Config, oldCfg *config.Config) error {
	oldByID := make(map[string]config.ModelConfig, len(oldCfg.Models))
	for _, model := range oldCfg.Models {
		oldByID[model.ID] = model
	}
	usedOriginalIDs := make(map[string]struct{}, len(newCfg.Models))
	for i := range newCfg.Models {
		model := &newCfg.Models[i]
		lookupID := model.OriginalID
		if lookupID == "" {
			lookupID = model.ID
		} else {
			if _, exists := usedOriginalIDs[lookupID]; exists {
				return fmt.Errorf("model original_id is duplicated: %s", lookupID)
			}
			usedOriginalIDs[lookupID] = struct{}{}
		}
		oldModel, exists := oldByID[lookupID]
		if model.OriginalID != "" && !exists {
			return fmt.Errorf("model original_id does not exist: %s", model.OriginalID)
		}
		if model.APIKey == "" && exists {
			model.APIKey = oldModel.APIKey
		} else if model.APIKey == "" && model.HasAPIKey {
			return fmt.Errorf("model api key cannot be restored: %s", model.ID)
		}
		if model.ExtraHeaders == sensitivePassword {
			if !exists {
				return fmt.Errorf("model extra headers cannot be restored: %s", model.ID)
			}
			model.ExtraHeaders = oldModel.ExtraHeaders
		}
		model.HasAPIKey = false
		model.OriginalID = ""
	}
	return nil
}

func restoreMaskedAPIKeys(submitted, existing []string) ([]string, error) {
	maskedExisting := make(map[string][]string, len(existing))
	for _, apiKey := range existing {
		masked := maskAPIKey(apiKey)
		maskedExisting[masked] = append(maskedExisting[masked], apiKey)
	}
	restored := make([]string, 0, len(submitted))
	for _, apiKey := range submitted {
		matches := maskedExisting[apiKey]
		switch len(matches) {
		case 0:
			restored = append(restored, apiKey)
		case 1:
			restored = append(restored, matches[0])
		default:
			return nil, fmt.Errorf("masked openai api key is ambiguous")
		}
	}
	return restored, nil
}

func userAgentConfigItems(items []config.AgentConfig) []config.AgentConfig {
	result := items[:0]
	for _, item := range items {
		if agents.IsRequiredBuiltinAgentID(item.ID) {
			continue
		}
		if item.Builtin || agents.IsBuiltinAgentID(item.ID) {
			result = append(result, config.AgentConfig{
				ID:      item.ID,
				Enabled: item.Enabled,
			})
			continue
		}
		result = append(result, item)
	}
	return result
}

func maskAgentSSHPasswords(items []config.AgentConfig) {
	for i := range items {
		if items[i].SSH != nil && items[i].SSH.Password != "" {
			items[i].SSH.Password = sensitivePassword
		}
	}
}

func restoreAgentSSHPasswords(items []config.AgentConfig, oldCfg *config.Config) {
	if oldCfg == nil {
		return
	}
	oldItems := agents.ConfigItems(oldCfg)
	oldByID := make(map[string]config.AgentConfig, len(oldItems))
	for _, item := range oldItems {
		if item.ID != "" {
			oldByID[item.ID] = item
		}
	}
	for i := range items {
		if items[i].SSH == nil || items[i].SSH.Password != sensitivePassword {
			continue
		}
		if oldItem, ok := oldByID[items[i].ID]; ok && oldItem.SSH != nil {
			items[i].SSH.Password = oldItem.SSH.Password
		}
	}
}

// resetMemoryLLM 使用当前配置重建 MemoryManager 的 LLM 客户端
func resetMemoryLLM(ctx context.Context, state *appstate.State, registry *modelregistry.Registry) {
	manager := memoryFromState(state)
	if manager == nil {
		return
	}
	cfg := config.Get()
	modelCfg := cfg.ResolveDefaultModel(config.ModelUseChat)
	if registry == nil || modelCfg == nil {
		log.Printf("[memory] model registry or default chat model is not configured")
		return
	}
	chatModel, err := registry.NewChatModel(ctx, &modelregistry.Config{
		Provider:     modelregistry.Type(modelCfg.Provider),
		APIKey:       modelCfg.APIKey,
		BaseURL:      modelCfg.BaseURL,
		Model:        modelCfg.Model,
		ExtraHeaders: modelCfg.ParseExtraHeaders(),
	})
	if err != nil {
		log.Printf("[memory] 重建模型失败，记忆服务继续使用旧模型: %v", err)
		return
	}
	llmClient, err := memorymodel.NewLLMClient(chatModel)
	if err != nil {
		log.Printf("[memory] 适配模型失败，记忆服务继续使用旧模型: %v", err)
		return
	}
	manager.ResetLLM(llmClient)
	log.Println("[memory] 记忆服务模型已更新")
}

// GetToolNamesHandler 获取可用工具名列表

func (rt *Runtime) GetToolNamesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rt.ToolRegistry == nil {
			OK(c, []string{})
			return
		}
		OK(c, rt.ToolRegistry.GetAllToolNames(c.Request.Context()))
	}
}

// GetToolCatalogHandler 获取可配置工具组详情

func (rt *Runtime) GetToolCatalogHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rt.ToolRegistry == nil {
			OK(c, []tools.ToolGroupInfo{})
			return
		}
		OK(c, rt.ToolRegistry.GetAllToolInfos(c.Request.Context()))
	}
}

// GetTemplateVarsHandler 返回可用的模板变量列表（供前端提示词编辑器补全）
func GetTemplateVarsHandler() gin.HandlerFunc {
	type templateVar struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Example     string `json:"example"`
	}
	vars := []templateVar{
		{Name: "os_type", Description: "操作系统类型", Example: runtime.GOOS},
		{Name: "os_arch", Description: "系统架构", Example: runtime.GOARCH},
		{Name: "workspace_dir", Description: "工作目录路径", Example: "/path/to/workspace"},
	}
	return func(c *gin.Context) {
		OK(c, vars)
	}
}

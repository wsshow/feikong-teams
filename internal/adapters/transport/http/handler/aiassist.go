package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	javascripthooks "fkteams/internal/adapters/hooks/javascript"
	javascripttool "fkteams/internal/adapters/tools/javascript"
	appaiassist "fkteams/internal/app/aiassist"
	"fkteams/internal/app/config"
	appskill "fkteams/internal/app/skill"
	apptools "fkteams/internal/app/tools"

	"github.com/gin-gonic/gin"
)

const statusClientClosedRequest = 499

func (rt *Runtime) GenerateAgentDraftsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		releaseExecution := rt.ExecutionGate.Begin()
		defer releaseExecution()
		var req appaiassist.AgentDraftRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		enrichAgentDraftRequest(ctx, &req, config.Get(), rt.ToolRegistry)
		service, err := appaiassist.NewDefault(ctx, rt.ModelRegistry)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := service.GenerateAgents(ctx, req)
		if err != nil {
			failAIAssistError(c, err)
			return
		}
		OK(c, resp)
	}
}

func (rt *Runtime) GenerateSkillDraftHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		releaseExecution := rt.ExecutionGate.Begin()
		defer releaseExecution()
		var req appaiassist.SkillDraftRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		enrichSkillDraftRequest(&req)
		service, err := appaiassist.NewDefault(ctx, rt.ModelRegistry)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := service.GenerateSkill(ctx, req)
		if err != nil {
			failAIAssistError(c, err)
			return
		}
		OK(c, resp)
	}
}

func (rt *Runtime) GenerateJavaScriptDraftHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		releaseExecution := rt.ExecutionGate.Begin()
		defer releaseExecution()
		var req appaiassist.JavaScriptDraftRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		enrichJavaScriptDraftRequest(ctx, &req, config.Get(), rt.ToolRegistry)
		service, err := appaiassist.NewDefault(ctx, rt.ModelRegistry)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := service.GenerateJavaScript(ctx, req)
		if err != nil {
			failAIAssistError(c, err)
			return
		}
		if err := validateJavaScriptDraft(resp); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		OK(c, resp)
	}
}

func enrichJavaScriptDraftRequest(ctx context.Context, req *appaiassist.JavaScriptDraftRequest, cfg *config.Config, registry *apptools.ToolGroupRegistry) {
	if req == nil || cfg == nil {
		return
	}
	if registry != nil {
		req.AvailableToolGroups = registry.GetAllToolInfos(ctx)
	}
	if len(req.ExistingIDs) > 0 {
		return
	}
	currentID := ""
	if req.Kind == "tool" && req.CurrentTool != nil {
		currentID = req.CurrentTool.ID
	}
	if req.Kind == "hook" && req.CurrentHook != nil {
		currentID = req.CurrentHook.ID
	}
	if req.Kind == "tool" {
		for _, item := range cfg.JavaScript.Tools {
			if item.ID != "" && item.ID != currentID {
				req.ExistingIDs = append(req.ExistingIDs, item.ID)
			}
		}
		return
	}
	for _, item := range cfg.JavaScript.Hooks {
		if item.ID != "" && item.ID != currentID {
			req.ExistingIDs = append(req.ExistingIDs, item.ID)
		}
	}
}

func validateJavaScriptDraft(resp appaiassist.JavaScriptDraftResponse) error {
	cfg := &config.Config{}
	if resp.Tool != nil {
		tool := *resp.Tool
		tool.Enabled = true
		cfg.JavaScript.Tools = []config.JavaScriptTool{tool}
	}
	if resp.Hook != nil {
		hook := *resp.Hook
		hook.Enabled = true
		cfg.JavaScript.Hooks = []config.JavaScriptHook{hook}
	}
	if err := cfg.ValidateJavaScript(); err != nil {
		return err
	}
	if err := javascripttool.ValidateDefinitions(cfg.JavaScript.Tools); err != nil {
		return err
	}
	return javascripthooks.ValidateDefinitions(cfg.JavaScript.Hooks)
}

func enrichSkillDraftRequest(req *appaiassist.SkillDraftRequest) {
	if req == nil || len(req.ExistingSkills) > 0 {
		return
	}
	skills, err := appskill.ListLocalSkills()
	if err != nil {
		return
	}
	for _, item := range skills {
		if item.Slug != "" {
			req.ExistingSkills = append(req.ExistingSkills, item.Slug)
		}
	}
}

func (rt *Runtime) RewriteTextHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		releaseExecution := rt.ExecutionGate.Begin()
		defer releaseExecution()
		var req appaiassist.RewriteTextRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		service, err := appaiassist.NewDefault(ctx, rt.ModelRegistry)
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := service.RewriteText(ctx, req)
		if err != nil {
			failAIAssistError(c, err)
			return
		}
		OK(c, resp)
	}
}

func failAIAssistError(c *gin.Context, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(c.Request.Context().Err(), context.Canceled) {
		Fail(c, statusClientClosedRequest, "client closed request")
		return
	}
	Fail(c, http.StatusBadRequest, err.Error())
}

func enrichAgentDraftRequest(ctx context.Context, req *appaiassist.AgentDraftRequest, cfg *config.Config, toolRegistry *apptools.ToolGroupRegistry) {
	if req == nil || cfg == nil {
		return
	}
	if len(req.ExistingAgents) == 0 {
		for _, agent := range cfg.Agents.Items {
			id := agent.ID
			if id == "" {
				id = agent.Name
			}
			if id != "" {
				req.ExistingAgents = append(req.ExistingAgents, id)
			}
		}
	}
	if len(req.AvailableModels) == 0 {
		for _, model := range cfg.Models {
			if model.ID != "" {
				req.AvailableModels = append(req.AvailableModels, model.ID)
			}
		}
	}
	if len(req.AvailableTools) == 0 {
		if toolRegistry != nil {
			req.AvailableTools = toolRegistry.GetAllToolNames(ctx)
		}
	}
	if req.DefaultModelID == "" {
		if model := cfg.ResolveDefaultModel(config.ModelUseChat); model != nil {
			req.DefaultModelID = model.ID
		}
	}
}

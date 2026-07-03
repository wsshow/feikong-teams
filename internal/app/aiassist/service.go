// Package aiassist provides reusable AI-assisted draft and rewrite use cases.
package aiassist

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"fkteams/internal/app/config"
	domainmessage "fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
	modelregistry "fkteams/internal/runtime/model"
)

const maxAgentDrafts = 10

type Service struct {
	model runtimeport.ChatModel
}

type AgentDraft struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	ModelID     string   `json:"model_id,omitempty"`
	Tools       []string `json:"tools"`
	Enabled     bool     `json:"enabled"`
}

type AgentDraftRequest struct {
	Instruction     string   `json:"instruction"`
	ExistingAgents  []string `json:"existing_agents"`
	AvailableTools  []string `json:"available_tools"`
	AvailableModels []string `json:"available_models"`
	DefaultModelID  string   `json:"default_model_id"`
}

type AgentDraftResponse struct {
	Agents []AgentDraft `json:"agents"`
}

type RewriteTextRequest struct {
	Scenario    string         `json:"scenario"`
	Instruction string         `json:"instruction"`
	Text        string         `json:"text"`
	Context     map[string]any `json:"context,omitempty"`
}

type RewriteTextResponse struct {
	Text string `json:"text"`
}

func New(model runtimeport.ChatModel) *Service {
	return &Service{model: model}
}

func NewDefault(ctx context.Context) (*Service, error) {
	cfg := config.Get()
	modelCfg := cfg.ResolveDefaultModel(config.ModelUseChat)
	if modelCfg == nil || (modelCfg.APIKey == "" && modelCfg.Provider == "") {
		return nil, fmt.Errorf("default chat model is not configured")
	}
	registry, err := modelregistry.RequireRegistry(ctx)
	if err != nil {
		return nil, err
	}
	model, err := registry.NewChatModel(ctx, &modelregistry.Config{
		Provider:     modelregistry.Type(modelCfg.Provider),
		APIKey:       modelCfg.APIKey,
		BaseURL:      modelCfg.BaseURL,
		Model:        modelCfg.Model,
		ExtraHeaders: modelCfg.ParseExtraHeaders(),
	})
	if err != nil {
		return nil, err
	}
	return New(model), nil
}

func (s *Service) GenerateAgents(ctx context.Context, req AgentDraftRequest) (AgentDraftResponse, error) {
	if s == nil || s.model == nil {
		return AgentDraftResponse{}, fmt.Errorf("ai assist model is not configured")
	}
	req.Instruction = strings.TrimSpace(req.Instruction)
	if req.Instruction == "" {
		return AgentDraftResponse{}, fmt.Errorf("instruction is required")
	}

	resp, err := s.model.Generate(ctx, []domainmessage.Message{
		{Role: domainmessage.RoleSystem, Content: agentDraftSystemPrompt()},
		{Role: domainmessage.RoleUser, Content: marshalPromptPayload(req)},
	})
	if err != nil {
		return AgentDraftResponse{}, err
	}

	var parsed AgentDraftResponse
	if err := decodeJSONResponse(resp.Content, &parsed); err != nil {
		return AgentDraftResponse{}, fmt.Errorf("decode agent drafts: %w", err)
	}
	parsed.Agents = normalizeAgentDrafts(parsed.Agents, req)
	if len(parsed.Agents) == 0 {
		return AgentDraftResponse{}, fmt.Errorf("model did not return valid agent drafts")
	}
	return parsed, nil
}

func (s *Service) RewriteText(ctx context.Context, req RewriteTextRequest) (RewriteTextResponse, error) {
	if s == nil || s.model == nil {
		return RewriteTextResponse{}, fmt.Errorf("ai assist model is not configured")
	}
	req.Instruction = strings.TrimSpace(req.Instruction)
	if req.Instruction == "" {
		return RewriteTextResponse{}, fmt.Errorf("instruction is required")
	}

	resp, err := s.model.Generate(ctx, []domainmessage.Message{
		{Role: domainmessage.RoleSystem, Content: rewriteSystemPrompt()},
		{Role: domainmessage.RoleUser, Content: marshalPromptPayload(req)},
	})
	if err != nil {
		return RewriteTextResponse{}, err
	}

	var parsed RewriteTextResponse
	if err := decodeJSONResponse(resp.Content, &parsed); err != nil {
		return RewriteTextResponse{}, fmt.Errorf("decode rewritten text: %w", err)
	}
	parsed.Text = strings.TrimSpace(parsed.Text)
	if parsed.Text == "" {
		return RewriteTextResponse{}, fmt.Errorf("model returned empty text")
	}
	return parsed, nil
}

func agentDraftSystemPrompt() string {
	return strings.TrimSpace(`
你是 fkteams 的智能体配置助手。根据用户要求生成一个或多个自定义智能体草稿。
必须只返回 JSON，不要返回 Markdown，不要解释。
JSON 格式必须是：
{
  "agents": [
    {
      "id": "agent_id",
      "name": "智能体名称",
      "description": "一句话描述",
      "prompt": "完整系统提示词",
      "model_id": "模型 ID，可为空",
      "tools": ["工具名"],
      "enabled": true
    }
  ]
}
要求：
- 最多生成 10 个智能体。
- id 使用小写英文、数字和下划线。
- description 简洁准确。
- prompt 必须能直接作为系统提示词使用，写清角色、职责、边界、输出风格和必要澄清策略。
- tools 只能从用户提供的 available_tools 中选择；不确定就返回空数组。
- 不要生成 SSH 密码、API Key 或其他敏感信息。
`)
}

func rewriteSystemPrompt() string {
	return strings.TrimSpace(`
你是可复用的 AI 文本编辑助手。根据用户要求改写给定文本。
必须只返回 JSON，不要返回 Markdown，不要解释。
JSON 格式必须是：
{
  "text": "改写后的完整文本"
}
要求：
- 保留用户明确要求保留的信息。
- 如果是系统提示词，输出必须可以直接作为系统提示词使用。
- 如果是描述，保持简洁、准确、适合界面展示。
- 不要添加用户没有要求的敏感信息、密钥、账号或密码。
`)
}

func marshalPromptPayload(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeJSONResponse(text string, target any) error {
	candidates := []string{strings.TrimSpace(text)}
	if fenced := extractFencedJSON(text); fenced != "" {
		candidates = append(candidates, fenced)
	}
	if object := extractJSONObject(text); object != "" {
		candidates = append(candidates, object)
	}
	var lastErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if err := json.Unmarshal([]byte(candidate), target); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("empty response")
}

var fencedJSONPattern = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

func extractFencedJSON(text string) string {
	match := fencedJSONPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(text[start : end+1])
}

func normalizeAgentDrafts(items []AgentDraft, req AgentDraftRequest) []AgentDraft {
	if len(items) > maxAgentDrafts {
		items = items[:maxAgentDrafts]
	}
	used := make(map[string]bool)
	for _, id := range req.ExistingAgents {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			used[trimmed] = true
		}
	}
	allowedTools := make(map[string]bool)
	for _, tool := range req.AvailableTools {
		if trimmed := strings.TrimSpace(tool); trimmed != "" {
			allowedTools[trimmed] = true
		}
	}
	defaultModel := strings.TrimSpace(req.DefaultModelID)
	if defaultModel == "" && len(req.AvailableModels) > 0 {
		defaultModel = strings.TrimSpace(req.AvailableModels[0])
	}

	normalized := make([]AgentDraft, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		item.Prompt = strings.TrimSpace(item.Prompt)
		baseID := firstNonEmpty(item.ID, item.Name, "agent")
		item.ID = uniqueSlug(baseID, used)
		if item.Name == "" {
			item.Name = item.ID
		}
		if item.Description == "" {
			item.Description = item.Name
		}
		if item.Prompt == "" {
			item.Prompt = fmt.Sprintf("你是%s。请根据用户需求提供准确、简洁、可执行的帮助；信息不足时先澄清关键问题。", item.Name)
		}
		if strings.TrimSpace(item.ModelID) == "" {
			item.ModelID = defaultModel
		}
		item.Tools = filterTools(item.Tools, allowedTools)
		item.Enabled = true
		normalized = append(normalized, item)
	}
	return normalized
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func uniqueSlug(value string, used map[string]bool) string {
	base := slugify(value)
	candidate := base
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	used[candidate] = true
	return candidate
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastSep := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		case r == '_' || r == '-' || unicode.IsSpace(r):
			if b.Len() > 0 && !lastSep {
				b.WriteByte('_')
				lastSep = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "agent"
	}
	return out
}

func filterTools(tools []string, allowed map[string]bool) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		trimmed := strings.TrimSpace(tool)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		if !allowed[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

// Package aiassist provides reusable AI-assisted draft and rewrite use cases.
package aiassist

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"fkteams/internal/app/config"
	apptools "fkteams/internal/app/tools"
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

type SkillDraft struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type SkillDraftRequest struct {
	Instruction    string   `json:"instruction"`
	ExistingSkills []string `json:"existing_skills"`
}

type SkillDraftResponse struct {
	Skill SkillDraft `json:"skill"`
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

type JavaScriptDraftRequest struct {
	Kind                string                   `json:"kind"`
	Instruction         string                   `json:"instruction"`
	ExistingIDs         []string                 `json:"existing_ids,omitempty"`
	AvailableToolGroups []apptools.ToolGroupInfo `json:"available_tool_groups,omitempty"`
	CurrentTool         *config.JavaScriptTool   `json:"current_tool,omitempty"`
	CurrentHook         *config.JavaScriptHook   `json:"current_hook,omitempty"`
}

type JavaScriptDraftResponse struct {
	Kind string                 `json:"kind"`
	Tool *config.JavaScriptTool `json:"tool,omitempty"`
	Hook *config.JavaScriptHook `json:"hook,omitempty"`
}

func New(model runtimeport.ChatModel) *Service {
	return &Service{model: model}
}

func NewDefault(ctx context.Context, registry *modelregistry.Registry) (*Service, error) {
	cfg := config.Get()
	modelCfg := cfg.ResolveDefaultModel(config.ModelUseChat)
	if modelCfg == nil || (modelCfg.APIKey == "" && modelCfg.Provider == "") {
		return nil, fmt.Errorf("default chat model is not configured")
	}
	if registry == nil {
		return nil, fmt.Errorf("model registry is not configured")
	}
	model, err := registry.NewChatModel(ctx, &modelregistry.Config{
		Provider:       modelregistry.Type(modelCfg.Provider),
		APIKey:         modelCfg.APIKey,
		BaseURL:        modelCfg.BaseURL,
		Model:          modelCfg.Model,
		ExtraHeaders:   modelCfg.ParseExtraHeaders(),
		SupportsVision: modelCfg.SupportsVision,
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

func (s *Service) GenerateSkill(ctx context.Context, req SkillDraftRequest) (SkillDraftResponse, error) {
	if s == nil || s.model == nil {
		return SkillDraftResponse{}, fmt.Errorf("ai assist model is not configured")
	}
	req.Instruction = strings.TrimSpace(req.Instruction)
	if req.Instruction == "" {
		return SkillDraftResponse{}, fmt.Errorf("instruction is required")
	}

	resp, err := s.model.Generate(ctx, []domainmessage.Message{
		{Role: domainmessage.RoleSystem, Content: skillDraftSystemPrompt()},
		{Role: domainmessage.RoleUser, Content: marshalPromptPayload(req)},
	})
	if err != nil {
		return SkillDraftResponse{}, err
	}

	var parsed SkillDraftResponse
	if err := decodeJSONResponse(resp.Content, &parsed); err != nil {
		return SkillDraftResponse{}, fmt.Errorf("decode skill draft: %w", err)
	}
	parsed.Skill = normalizeSkillDraft(parsed.Skill, req)
	if parsed.Skill.Content == "" {
		return SkillDraftResponse{}, fmt.Errorf("model did not return valid skill draft")
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

// GenerateJavaScript 根据自然语言生成可供 goja 执行的工具或 hook 草稿。
func (s *Service) GenerateJavaScript(ctx context.Context, req JavaScriptDraftRequest) (JavaScriptDraftResponse, error) {
	if s == nil || s.model == nil {
		return JavaScriptDraftResponse{}, fmt.Errorf("ai assist model is not configured")
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.Instruction = strings.TrimSpace(req.Instruction)
	if req.Kind != "tool" && req.Kind != "hook" {
		return JavaScriptDraftResponse{}, fmt.Errorf("javascript draft kind must be tool or hook")
	}
	if req.Instruction == "" {
		return JavaScriptDraftResponse{}, fmt.Errorf("instruction is required")
	}

	resp, err := s.model.Generate(ctx, []domainmessage.Message{
		{Role: domainmessage.RoleSystem, Content: javaScriptDraftSystemPrompt()},
		{Role: domainmessage.RoleUser, Content: marshalPromptPayload(req)},
	})
	if err != nil {
		return JavaScriptDraftResponse{}, err
	}

	var parsed JavaScriptDraftResponse
	if err := decodeJSONResponse(resp.Content, &parsed); err != nil {
		return JavaScriptDraftResponse{}, fmt.Errorf("decode javascript draft: %w", err)
	}
	return normalizeJavaScriptDraft(parsed, req)
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

func skillDraftSystemPrompt() string {
	return strings.TrimSpace(`
你是 fkteams 的 Skills 创建助手。根据用户要求生成一个可直接安装到本地的自定义 Skill。
必须只返回 JSON，不要返回 Markdown，不要解释。
JSON 格式必须是：
{
  "skill": {
    "slug": "skill_slug",
    "name": "技能名称",
    "description": "一句话描述",
    "content": "完整 SKILL.md 文件内容"
  }
}
要求：
- slug 使用小写英文、数字和下划线。
- name 和 description 简洁准确。
- content 必须是完整 SKILL.md，包含 YAML frontmatter：name、description。
- content 要写清使用场景、输入要求、执行步骤、质量标准和必要示例。
- 不要生成 SSH 密码、API Key、令牌或其他敏感信息。
- 不要引用不存在的本地文件。
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

func javaScriptDraftSystemPrompt() string {
	return strings.TrimSpace(`
你是 fkteams 的 JavaScript 扩展生成器。根据用户要求生成一个可由 dop251/goja 同步执行的工具或流程 hook。
必须只返回 JSON，不要返回 Markdown，不要解释，也不要把源码放进代码围栏。

工具格式：
{
  "kind": "tool",
  "tool": {
    "id": "lower_snake_case_id",
    "name": "展示名称",
    "description": "供模型理解用途和调用时机的准确描述",
    "enabled": false,
    "timeout_ms": 200,
    "read_only": true,
    "parameters": {"type":"object","properties":{},"required":[]},
    "source": "function execute(input, context) { return ...; }"
  }
}

Hook 格式：
{
  "kind": "hook",
  "hook": {
    "id": "lower_snake_case_id",
    "name": "展示名称",
    "enabled": false,
    "hook_points": ["before_tool_call"],
    "timeout_ms": 200,
    "error_policy": "fail",
    "priority": 100,
    "source": "function handle(hook) { return {action: 'continue'}; }"
  }
}

执行环境约束：
- 只支持同步 JavaScript，不要使用 async、Promise、setTimeout、fetch、require、process、文件系统或 Node.js API。
- 每次调用使用独立 Runtime，不要依赖全局状态跨调用保存。
- 工具 execute 的 input 是 parameters 描述的 JSON 对象；context 顶层包含 tool_name、call_id、session_id。返回字符串、数字、布尔值、数组或对象。
- 优先使用简洁的宿主函数：context.workspace.read(path, options?)、write(path, content)、append(path, content)、replace(path, oldText, newText)、patch(diff)、list(path)、glob(pattern, options?)、grep(pattern, options?)。
- context.workspace.read 可选 start_line、end_line；glob 可选 path；grep 可选 path、include、use_regex、context、max_count。
- context.shell.run(command, options?) 执行命令，可选 timeout、reason、background、task_id、task_action。
- context.web.search(query, options?) 返回搜索结果数组，可选 time_range；context.web.fetch(url, options?) 返回网页正文，可选 format、timeout。
- context.state 提供 get(key)、set(key,value)、delete(key)、keys()；context.notify(message, level?) 发送通知，level 可为 info、warn、error。
- context.log.debug/info/warn/error(value) 写入服务日志。
- 源码较长时可在 execute 参数中解构需要的能力，例如 function execute(input, { workspace, state })，后续直接调用 workspace.read(...) 和 state.get(...)。
- 只有宿主函数无法覆盖需求时，才使用 context.tools.call(group, tool, args) 调用 available_tool_groups 中的工具；禁止调用 javascript 工具组，也不要猜测不存在的组或工具名。
- workspace、shell、web 便捷函数会在底层工具返回 error_message 时直接抛出异常。需要降级处理时使用 try/catch。
- 宿主函数已经自动接入，不要生成权限字段或权限检查代码。
- handle 返回 {action, message, payload}；action 只能是 continue、skip、reject。需要改写时，修改 hook.payload 后把它作为 payload 返回。
- hook.point 可选值：before_run、after_run、on_event、before_tool_call、after_tool_call、before_model_request、after_model_response。
- before_run payload 为 {input:{context,message}}；on_event 为 {event}；before_tool_call 为 {tool_name,args,meta}；before_model_request 为 {messages,meta}。
- after_run、after_tool_call、after_model_response 只用于观察，返回的 payload 不会改写已发生的结果。
- args 是 JSON 字符串，改写工具参数时必须再次 JSON.stringify。
- 不要生成密钥、密码或假设存在未声明的宿主 API。
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

func normalizeSkillDraft(item SkillDraft, req SkillDraftRequest) SkillDraft {
	used := make(map[string]bool)
	for _, slug := range req.ExistingSkills {
		if trimmed := strings.TrimSpace(slug); trimmed != "" {
			used[trimmed] = true
		}
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.Content = strings.TrimSpace(item.Content)
	item.Slug = uniqueSlug(firstNonEmpty(item.Slug, item.Name, "custom_skill"), used)
	if item.Name == "" {
		item.Name = item.Slug
	}
	if item.Description == "" {
		item.Description = item.Name
	}
	if item.Content == "" {
		item.Content = defaultSkillDraftContent(item.Name, item.Description)
	}
	return item
}

func normalizeJavaScriptDraft(parsed JavaScriptDraftResponse, req JavaScriptDraftRequest) (JavaScriptDraftResponse, error) {
	used := make(map[string]bool)
	for _, id := range req.ExistingIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			used[trimmed] = true
		}
	}
	parsed.Kind = req.Kind
	if req.Kind == "tool" {
		if parsed.Tool == nil {
			return JavaScriptDraftResponse{}, fmt.Errorf("model did not return a javascript tool")
		}
		item := *parsed.Tool
		item.ID = uniqueJavaScriptID(firstNonEmpty(item.ID, item.Name, "custom_tool"), used)
		item.Name = firstNonEmpty(item.Name, item.ID)
		item.Description = firstNonEmpty(item.Description, item.Name)
		item.Source = cleanJavaScriptSource(item.Source)
		item.Enabled = false
		item.TimeoutMS = normalizeJavaScriptTimeout(item.TimeoutMS, 120_000)
		if schemaType, _ := item.Parameters["type"].(string); schemaType != "object" {
			item.Parameters = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		if item.Source == "" {
			return JavaScriptDraftResponse{}, fmt.Errorf("model returned empty javascript tool source")
		}
		return JavaScriptDraftResponse{Kind: "tool", Tool: &item}, nil
	}

	if parsed.Hook == nil {
		return JavaScriptDraftResponse{}, fmt.Errorf("model did not return a javascript hook")
	}
	item := *parsed.Hook
	item.ID = uniqueJavaScriptID(firstNonEmpty(item.ID, item.Name, "custom_hook"), used)
	item.Name = firstNonEmpty(item.Name, item.ID)
	item.Source = cleanJavaScriptSource(item.Source)
	item.Enabled = false
	item.TimeoutMS = normalizeJavaScriptTimeout(item.TimeoutMS, 5_000)
	item.HookPoints = validGeneratedHookPoints(item.HookPoints)
	if len(item.HookPoints) == 0 {
		item.HookPoints = []string{"before_tool_call"}
	}
	if item.ErrorPolicy != "ignore" && item.ErrorPolicy != "warn" && item.ErrorPolicy != "fail" {
		item.ErrorPolicy = "fail"
	}
	if item.Source == "" {
		return JavaScriptDraftResponse{}, fmt.Errorf("model returned empty javascript hook source")
	}
	return JavaScriptDraftResponse{Kind: "hook", Hook: &item}, nil
}

func uniqueJavaScriptID(value string, used map[string]bool) string {
	base := slugify(value)
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = "js_" + base
	}
	return uniqueSlug(base, used)
}

func normalizeJavaScriptTimeout(timeoutMS, maximum int) int {
	if timeoutMS <= 0 {
		return 200
	}
	if timeoutMS > maximum {
		return maximum
	}
	return timeoutMS
}

func validGeneratedHookPoints(points []string) []string {
	valid := map[string]bool{
		"before_run":           true,
		"after_run":            true,
		"on_event":             true,
		"before_tool_call":     true,
		"after_tool_call":      true,
		"before_model_request": true,
		"after_model_response": true,
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(points))
	for _, point := range points {
		point = strings.TrimSpace(point)
		if valid[point] && !seen[point] {
			seen[point] = true
			result = append(result, point)
		}
	}
	return result
}

func cleanJavaScriptSource(source string) string {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "```") && strings.HasSuffix(source, "```") {
		lines := strings.Split(source, "\n")
		if len(lines) >= 3 {
			lines = lines[1 : len(lines)-1]
			source = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	return source
}

func defaultSkillDraftContent(name, description string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "---\nname: %s\ndescription: %s\n---\n\n", strconv.Quote(name), strconv.Quote(description))
	fmt.Fprintf(&sb, "# %s\n\n%s\n\n", name, description)
	sb.WriteString("## Use when\n\n- Use this skill when the task matches the user's stated goal.\n\n")
	sb.WriteString("## Instructions\n\n- Clarify missing requirements before acting.\n- Follow the project's existing constraints.\n- Provide concise verification notes when finished.\n")
	return strings.TrimSpace(sb.String())
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

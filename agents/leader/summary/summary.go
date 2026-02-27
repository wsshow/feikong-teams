package summary

import (
	"context"
	"fkteams/fkevent"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const DefaultMaxTokensBeforeSummary = 80 * 1024
const DefaultMaxTokensForRecentMessages = 25 * 1024 // 20% of DefaultMaxTokensBeforeSummary
const PromptOfSummary = `<role>
Conversation Summarization Assistant for Multi-turn LLM Agent
</role>

<primary_objective>
Summarize the older portion of the conversation history into a concise, accurate, and information-rich context summary. 
The summary must preserve essential reasoning, actions, outcomes, and lessons learned, 
allowing the agent to continue reasoning seamlessly without re-accessing the raw conversation data.
</primary_objective>

<contextual_goals>
- Include major progress, decisions made, reasoning steps, intermediate or final results, and lessons (both successes and failures).
- Emphasize failed attempts, misunderstandings, and improvements or adjustments that followed.
- Exclude irrelevant details, casual talk, and redundant confirmations.
- Maintain consistency with the current System Prompt and the user’s long-term goals.
</contextual_goals>

<instructions>
1. You will receive five tagged sections:
   - The **system_prompt tag** — provides the current System Prompt (for reference only, do not summarize).
   - The **user_messages tag** — contains early or persistent user instructions, preferences, and goals. Use it to maintain alignment with the user's long-term intent(for reference only, do not summarize).
   - The **previous_summary tag** — contains the existing long-term summary, if available.
   - The **older_messages tag** — includes earlier conversation messages to be summarized.
   - The **recent_messages tag** — contains the most recent conversation window (for reference only, do not summarize).

2. Your task:
   - Merge the content from the previous_summary tag and the older_messages tag into a new refined long-term summary.
   - When summarizing, integrate the key takeaways, decisions, lessons, and relevant state information.
   - Use the user_messages tag to ensure the summary preserves the user's persistent intent and constraints (ignore transient chit-chat).
   - Use the recent_messages tag only to maintain temporal and contextual continuity across turns.

3. Output requirements:
   - Respond **only** with the updated long-term summary that replaces the older conversation history.
   - Do **not** include any extra headers, XML tags, or meta explanations in your output.
</instructions>

<messages>
<system_prompt>
{system_prompt}
</system_prompt>

<user_messages>
{user_messages}
</user_messages>

<previous_summary>
{previous_summary}
</previous_summary>

<older_messages>
{older_messages}
</older_messages>

<recent_messages>
{recent_messages}
</recent_messages>
</messages>`

type TokenCounter func(ctx context.Context, msgs []adk.Message) (tokenNum []int64, err error)

// SummaryPersistCallback 摘要持久化回调
type SummaryPersistCallback func(summaryText string)

type summaryPersistCallbackKey struct{}

// WithSummaryPersistCallback 设置摘要持久化回调到 context
func WithSummaryPersistCallback(ctx context.Context, cb SummaryPersistCallback) context.Context {
	return context.WithValue(ctx, summaryPersistCallbackKey{}, cb)
}

type Config struct {
	MaxTokensBeforeSummary     int
	MaxTokensForRecentMessages int
	Counter                    TokenCounter
	Model                      model.BaseChatModel
	SystemPrompt               string
}

func New(ctx context.Context, cfg *Config) (adk.AgentMiddleware, error) {
	if cfg == nil {
		return adk.AgentMiddleware{}, fmt.Errorf("config is nil")
	}

	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = PromptOfSummary
	}
	maxBefore := DefaultMaxTokensBeforeSummary
	if cfg.MaxTokensBeforeSummary > 0 {
		maxBefore = cfg.MaxTokensBeforeSummary
	}
	maxRecent := DefaultMaxTokensForRecentMessages
	if cfg.MaxTokensForRecentMessages > 0 {
		maxRecent = cfg.MaxTokensForRecentMessages
	}

	tpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("summarize 'older_messages': "))

	summarizer, err := compose.NewChain[map[string]any, *schema.Message]().
		AppendChatTemplate(tpl).
		AppendChatModel(cfg.Model).
		Compile(ctx, compose.WithGraphName("Summarizer"))
	if err != nil {
		return adk.AgentMiddleware{}, fmt.Errorf("compile summarizer failed, err=%w", err)
	}

	sm := &summaryMiddleware{
		counter:    defaultCounterToken,
		maxBefore:  maxBefore,
		maxRecent:  maxRecent,
		summarizer: summarizer,
	}
	if cfg.Counter != nil {
		sm.counter = cfg.Counter
	}
	return adk.AgentMiddleware{BeforeChatModel: sm.BeforeModel}, nil
}

const summaryMessageFlag = "_agent_middleware_summary_message"

type summaryMiddleware struct {
	counter   TokenCounter
	maxBefore int
	maxRecent int

	summarizer compose.Runnable[map[string]any, *schema.Message]
}

func (s *summaryMiddleware) BeforeModel(ctx context.Context, state *adk.ChatModelAgentState) (err error) {
	if state == nil || len(state.Messages) == 0 {
		return nil
	}

	messages := state.Messages
	msgsToken, err := s.counter(ctx, messages)
	if err != nil {
		return fmt.Errorf("count token failed, err=%w", err)
	}
	if len(messages) != len(msgsToken) {
		return fmt.Errorf("token count mismatch, msgNum=%d, tokenCountNum=%d", len(messages), len(msgsToken))
	}

	var total int64
	for _, t := range msgsToken {
		total += t
	}
	// Trigger summarization only when exceeding threshold
	if total <= int64(s.maxBefore) {
		return nil
	}

	// Build blocks with user-messages, summary-message, tool-call pairings
	type block struct {
		msgs   []*schema.Message
		tokens int64
	}
	idx := 0

	systemBlock := block{}
	if idx < len(messages) {
		m := messages[idx]
		if m != nil && m.Role == schema.System {
			systemBlock.msgs = append(systemBlock.msgs, m)
			systemBlock.tokens += msgsToken[idx]
			idx++
		}
	}
	userBlock := block{}
	for idx < len(messages) {
		m := messages[idx]
		if m == nil {
			idx++
			continue
		}
		if m.Role != schema.User {
			break
		}
		userBlock.msgs = append(userBlock.msgs, m)
		userBlock.tokens += msgsToken[idx]
		idx++
	}
	summaryBlock := block{}
	if idx < len(messages) {
		m := messages[idx]
		if m != nil && m.Role == schema.Assistant {
			if _, ok := m.Extra[summaryMessageFlag]; ok {
				summaryBlock.msgs = append(summaryBlock.msgs, m)
				summaryBlock.tokens += msgsToken[idx]
				idx++
			}
		}
	}

	toolBlocks := make([]block, 0)
	for i := idx; i < len(messages); i++ {
		m := messages[i]
		if m == nil {
			continue
		}
		if m.Role == schema.Assistant && len(m.ToolCalls) > 0 {
			b := block{msgs: []*schema.Message{m}, tokens: msgsToken[i]}
			// Collect subsequent tool messages matching any tool call id
			callIDs := make(map[string]struct{}, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				callIDs[tc.ID] = struct{}{}
			}
			j := i + 1
			for j < len(messages) {
				nm := messages[j]
				if nm == nil || nm.Role != schema.Tool {
					break
				}
				// Match by ToolCallID when available; if empty, include but keep boundary
				if nm.ToolCallID == "" {
					b.msgs = append(b.msgs, nm)
					b.tokens += msgsToken[j]
				} else {
					if _, ok := callIDs[nm.ToolCallID]; !ok {
						// Tool message not belonging to this assistant call -> end pairing
						break
					}
					b.msgs = append(b.msgs, nm)
					b.tokens += msgsToken[j]
				}
				j++
			}
			toolBlocks = append(toolBlocks, b)
			i = j - 1
			continue
		}
		toolBlocks = append(toolBlocks, block{msgs: []*schema.Message{m}, tokens: msgsToken[i]})
	}

	// Split into recent and older within token budget, from newest to oldest
	var recentBlocks []block
	var olderBlocks []block
	var recentTokens int64
	for i := len(toolBlocks) - 1; i >= 0; i-- {
		b := toolBlocks[i]
		if recentTokens+b.tokens > int64(s.maxRecent) {
			olderBlocks = append([]block{b}, olderBlocks...)
			continue
		}
		recentBlocks = append([]block{b}, recentBlocks...)
		recentTokens += b.tokens
	}

	joinBlocks := func(bs []block) string {
		var sb strings.Builder
		for _, b := range bs {
			for _, m := range b.msgs {
				sb.WriteString(renderMsg(m))
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	olderText := joinBlocks(olderBlocks)
	recentText := joinBlocks(recentBlocks)

	// 通知上下文压缩开始
	_ = fkevent.DispatchEvent(ctx, fkevent.Event{
		Type:       "action",
		AgentName:  "系统",
		ActionType: "context_compress_start",
		Content:    "对话上下文压缩中...",
	})

	msg, err := s.summarizer.Invoke(ctx, map[string]any{
		"system_prompt":    joinBlocks([]block{systemBlock}),
		"user_messages":    joinBlocks([]block{userBlock}),
		"previous_summary": joinBlocks([]block{summaryBlock}),
		"older_messages":   olderText,
		"recent_messages":  recentText,
	})
	if err != nil {
		return fmt.Errorf("summarize failed, err=%w", err)
	}

	summaryMsg := schema.AssistantMessage(msg.Content, nil)
	msg.Name = "summary"
	summaryMsg.Extra = map[string]any{
		summaryMessageFlag: true,
	}

	// 持久化摘要
	if cb, ok := ctx.Value(summaryPersistCallbackKey{}).(SummaryPersistCallback); ok {
		cb(msg.Content)
	}

	// 通知上下文压缩已触发
	_ = fkevent.DispatchEvent(ctx, fkevent.Event{
		Type:       "action",
		AgentName:  "系统",
		ActionType: "context_compress",
		Content:    "对话上下文已压缩，旧消息已被总结摘要替代",
		Detail:     msg.Content,
	})

	// Build new state: prepend summary message, keep recent messages
	newMessages := make([]*schema.Message, 0, len(messages))
	newMessages = append(newMessages, systemBlock.msgs...)
	newMessages = append(newMessages, userBlock.msgs...)
	newMessages = append(newMessages, summaryMsg)
	for _, b := range recentBlocks {
		newMessages = append(newMessages, b.msgs...)
	}

	state.Messages = newMessages
	return nil
}

// Render messages into strings
func renderMsg(m *schema.Message) string {
	if m == nil {
		return ""
	}
	var sb strings.Builder
	if m.Role == schema.Tool {
		if m.ToolName != "" {
			sb.WriteString("[tool:")
			sb.WriteString(m.ToolName)
			sb.WriteString("]\n")
		} else {
			sb.WriteString("[tool]\n")
		}
	} else {
		sb.WriteString("[")
		sb.WriteString(string(m.Role))
		sb.WriteString("]\n")
	}
	if m.Content != "" {
		sb.WriteString(m.Content)
		sb.WriteString("\n")
	}
	if m.Role == schema.Assistant && len(m.ToolCalls) > 0 {
		for _, tc := range m.ToolCalls {
			if tc.Function.Name != "" {
				sb.WriteString("tool_call: ")
				sb.WriteString(tc.Function.Name)
				sb.WriteString("\n")
			}
			if tc.Function.Arguments != "" {
				sb.WriteString("args: ")
				sb.WriteString(tc.Function.Arguments)
				sb.WriteString("\n")
			}
		}
	}
	for _, part := range m.UserInputMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			sb.WriteString(part.Text)
			sb.WriteString("\n")
		}
	}
	for _, part := range m.AssistantGenMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			sb.WriteString(part.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func defaultCounterToken(_ context.Context, msgs []adk.Message) ([]int64, error) {
	result := make([]int64, len(msgs))
	for i, m := range msgs {
		if m == nil {
			result[i] = 0
			continue
		}
		result[i] = estimateTokens(m)
	}
	return result, nil
}

// estimateTokens 使用启发式方法估算 token 数量。
// 规则：每4个ASCII字节 ≈ 1 token，每个非ASCII字符（如中文）≈ 1 token，
// 每条消息固定开销 4 tokens（role + framing）。
func estimateTokens(m *schema.Message) int64 {
	const overhead = 4 // per-message role/framing overhead

	var n int64 = overhead
	n += countTokensInString(m.Content)

	for _, tc := range m.ToolCalls {
		n += countTokensInString(tc.Function.Name)
		n += countTokensInString(tc.Function.Arguments)
		n += 3 // tool call framing
	}
	if m.ToolCallID != "" {
		n += countTokensInString(m.ToolCallID)
	}
	if m.ToolName != "" {
		n += countTokensInString(m.ToolName)
	}

	for _, part := range m.UserInputMultiContent {
		if part.Type == schema.ChatMessagePartTypeText {
			n += countTokensInString(part.Text)
		}
	}
	for _, part := range m.AssistantGenMultiContent {
		if part.Type == schema.ChatMessagePartTypeText {
			n += countTokensInString(part.Text)
		}
	}

	return n
}

// countTokensInString 分别统计 ASCII 字节和非 ASCII 字符。
// ASCII 字节 4个 ≈ 1 token；非ASCII字符 每个 ≈ 1 token。
func countTokensInString(s string) int64 {
	if s == "" {
		return 0
	}
	var ascii, nonASCII int64
	for _, r := range s {
		if r < 128 {
			ascii++
		} else {
			nonASCII++
		}
	}
	return ascii/4 + nonASCII
}

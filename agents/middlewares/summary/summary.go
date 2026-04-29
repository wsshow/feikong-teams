package summary

import (
	"context"
	"fkteams/fkevent"
	"fkteams/providers/copilot"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const DefaultMaxTokensBeforeSummary = 800 * 1024
const PromptOfSummary = `关键提示：仅以纯文本响应，不要调用任何工具。
- 你已拥有上述对话中所需的全部上下文，无需再读取文件或执行命令。
- 工具调用将被拒绝，并会浪费你唯一的回复机会——你将无法完成任务。
- 你的整个回复必须是纯文本：先输出一个 <analysis> 分析块，后面紧跟一个 <summary> 总结块。

你的任务是对目前为止的对话创建一份详细的总结，重点关注用户的明确请求和你之前的操作。
总结应全面捕捉技术细节、代码模式和架构决策，以便在不丢失上下文的情况下继续工作。

在输出最终总结之前，将分析过程放入 <analysis> 标签中，整理思路并确保覆盖所有必要要点：

1. 按时间顺序分析对话中的每条消息，对每个章节详细识别：
   - 用户的明确请求与意图
   - 你应对用户请求所采取的方法
   - 关键决策、技术概念和代码模式
   - 具体细节：文件名、完整代码片段、函数签名、文件编辑内容
   - 遇到的错误及修复方式
   - 特别关注用户的具体反馈，尤其是用户要求以不同方式处理的情况

2. 仔细核查技术准确性与完整性，确保每个必要要素都得到充分处理。

总结应包含以下章节：

1. **主要请求与意图**：详细记录用户的所有明确请求和意图
2. **关键技术概念**：列举讨论中涉及的所有重要技术概念、技术栈和框架
3. **文件与代码片段**：列举审查、修改或创建的具体文件和代码片段，特别关注最近的消息，在适用时包含完整代码片段，并说明该文件被读取或编辑的重要原因
4. **错误与修复**：列举所有遇到的错误及其修复方式，特别关注用户的具体反馈，尤其是用户要求以不同方式处理的情况
5. **问题解决**：记录已解决的问题及任何正在进行的故障排查
6. **所有用户消息**：列举所有非工具结果的用户消息，并将其包裹在 <all_user_messages>...</all_user_messages> 标签中，这些消息对于理解用户反馈和意图变化至关重要
7. **待处理任务**：列出明确要求你处理的所有待处理任务
8. **当前工作**：详细描述此总结请求前正在进行的工作，特别关注用户和助手的最近消息，在适用时包含文件名和代码片段
9. **可选的下一步**：列出与最近工作相关的下一步操作。确保该步骤与用户最近的明确请求及此总结请求前正在处理的任务直接相关。如果上一个任务已完成，仅在明确符合用户请求时才列出下一步，不要在未与用户确认的情况下开始处理无关请求。如果有下一步，请包含最近对话的直接引用，逐字引用以避免任务理解偏离

以下是输出结构示例：

<example>
<analysis>
[你的思考过程，确保全面准确地覆盖所有要点]
</analysis>
<summary>
1. 主要请求与意图：
   [详细描述]

2. 关键技术概念：
   - [概念 1]
   - [概念 2]

3. 文件与代码片段：
   - [文件名 1]
      - [该文件重要性说明]
      - [对文件所做更改的说明（如有）]
      - [重要代码片段]

4. 错误与修复：
   - [错误 1 的详细描述]：
      - [修复方式]

5. 问题解决：
   [已解决问题及正在进行的故障排查描述]

6. 所有用户消息：
<all_user_messages>
   - [详细的非工具用户消息]
</all_user_messages>

7. 待处理任务：
   - [任务 1]

8. 当前工作：
   [详细描述此总结请求前正在进行的工作]

9. 可选的下一步：
   [与最近工作直接相关的下一步，包含对话的直接引用]

</summary>
</example>

提醒：不要调用任何工具，仅以纯文本响应——先输出 <analysis> 块，再输出 <summary> 块。工具调用将被拒绝，你将无法完成任务。

---

以下是你将收到的五个标记章节：

- **system_prompt**：当前系统提示（仅供参考，不要总结）
- **user_messages**：用户的早期或持续性指令、偏好和目标（仅供参考，不要总结，但要确保总结与用户长期意图保持一致）
- **previous_summary**：已有的长期摘要（如有）
- **older_messages**：需要被总结的早期对话消息
- **recent_messages**：最近的对话窗口（仅供参考，不要总结，仅用于保持时序和上下文连续性）

你的任务：将 previous_summary 和 older_messages 的内容合并为一份新的、精炼的长期摘要。总结时整合关键要点、决策、经验和相关状态信息，使用 user_messages 确保总结保留用户的持久意图和约束（忽略短暂的闲聊），使用 recent_messages 仅用于保持跨轮次的时序和上下文连续性。

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
	MaxTokensSummarizerInput   int // 压缩模型最大输入 token 数，防止超出模型上下文窗口
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
	// 由 maxBefore 自动推导：recent 窗口占 20%，压缩模型输入限制等于触发阈值
	maxRecent := maxBefore / 5
	if cfg.MaxTokensForRecentMessages > 0 {
		maxRecent = cfg.MaxTokensForRecentMessages
	}
	maxSummarizerInput := maxBefore
	if cfg.MaxTokensSummarizerInput > 0 {
		maxSummarizerInput = cfg.MaxTokensSummarizerInput
	}

	tpl := prompt.FromMessages(schema.FString,
		schema.SystemMessage(systemPrompt),
		schema.UserMessage("请根据以上五个标记章节，将 older_messages 与 previous_summary 合并为新的长期摘要："))

	summarizer, err := compose.NewChain[map[string]any, *schema.Message]().
		AppendChatTemplate(tpl).
		AppendChatModel(cfg.Model).
		Compile(ctx, compose.WithGraphName("Summarizer"))
	if err != nil {
		return adk.AgentMiddleware{}, fmt.Errorf("compile summarizer failed, err=%w", err)
	}

	sm := &summaryMiddleware{
		counter:            defaultCounterToken,
		maxBefore:          maxBefore,
		maxRecent:          maxRecent,
		maxSummarizerInput: maxSummarizerInput,
		summarizer:         summarizer,
	}
	if cfg.Counter != nil {
		sm.counter = cfg.Counter
	}
	return adk.AgentMiddleware{BeforeChatModel: sm.BeforeModel}, nil
}

const summaryMessageFlag = "_fkteams_agent_middleware_summary_message"

type summaryMiddleware struct {
	counter            TokenCounter
	maxBefore          int
	maxRecent          int
	maxSummarizerInput int

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
				sb.WriteString(renderMsg(m, 0))
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	// 单条消息截断阈值: maxBefore/4
	maxContentPerMsg := s.maxBefore / 4
	joinBlocksTruncated := func(bs []block) string {
		var sb strings.Builder
		for _, b := range bs {
			for _, m := range b.msgs {
				sb.WriteString(renderMsg(m, maxContentPerMsg))
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	// 截断 olderBlocks 确保压缩模型输入不超出上下文限制
	// userBlock 也使用截断渲染，防止超长初始消息永久占用 token 预算
	userBlockText := joinBlocksTruncated([]block{userBlock})
	overheadTokens := countTokensInString(joinBlocks([]block{systemBlock})) +
		countTokensInString(userBlockText) +
		countTokensInString(joinBlocks([]block{summaryBlock})) +
		recentTokens + 1024 // 1024 为 prompt 模板开销
	maxOlderTokens := int64(s.maxSummarizerInput) - overheadTokens
	if maxOlderTokens < 1024 {
		maxOlderTokens = 1024
	}
	var olderTokensTotal int64
	for _, b := range olderBlocks {
		olderTokensTotal += b.tokens
	}
	for olderTokensTotal > maxOlderTokens && len(olderBlocks) > 1 {
		olderTokensTotal -= olderBlocks[0].tokens
		olderBlocks = olderBlocks[1:]
	}

	olderText := joinBlocksTruncated(olderBlocks)
	// 最终安全截断：若 olderText 估算 token 仍超出预算，从头部截断保留尾部内容
	if olderTextEst := countTokensInString(olderText); olderTextEst > maxOlderTokens {
		ratio := float64(maxOlderTokens) / float64(olderTextEst)
		cutStart := int(float64(len(olderText)) * (1.0 - ratio))
		for cutStart < len(olderText) && olderText[cutStart]&0xC0 == 0x80 {
			cutStart++
		}
		olderText = "...[部分早期内容已截断]\n" + olderText[cutStart:]
	}
	recentText := joinBlocksTruncated(recentBlocks)

	// 通知上下文压缩开始
	_ = fkevent.DispatchEvent(ctx, fkevent.Event{
		Type:       fkevent.EventAction,
		AgentName:  "系统",
		ActionType: fkevent.ActionContextCompressStart,
		Content:    "对话上下文压缩中...",
	})

	msg, err := s.summarizer.Invoke(copilot.WithAgentInitiator(ctx), map[string]any{
		"system_prompt":    joinBlocks([]block{systemBlock}),
		"user_messages":    userBlockText,
		"previous_summary": joinBlocks([]block{summaryBlock}),
		"older_messages":   olderText,
		"recent_messages":  recentText,
	})
	if err != nil {
		return fmt.Errorf("summarize failed, err=%w", err)
	}

	summaryMsg := schema.AssistantMessage(msg.Content, nil)
	summaryMsg.Name = "summary"
	summaryMsg.Extra = map[string]any{
		summaryMessageFlag: true,
	}

	// 持久化摘要
	if cb, ok := ctx.Value(summaryPersistCallbackKey{}).(SummaryPersistCallback); ok {
		cb(msg.Content)
	}

	// 通知上下文压缩已触发
	_ = fkevent.DispatchEvent(ctx, fkevent.Event{
		Type:       fkevent.EventAction,
		AgentName:  "系统",
		ActionType: fkevent.ActionContextCompress,
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

// renderMsg renders a message to string. If maxContent > 0, truncates content exceeding that token count.
func renderMsg(m *schema.Message, maxContent int) string {
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
		sb.WriteString(truncateContent(m.Content, maxContent))
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
				sb.WriteString(truncateContent(tc.Function.Arguments, maxContent))
				sb.WriteString("\n")
			}
		}
	}
	for _, part := range m.UserInputMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			sb.WriteString(truncateContent(part.Text, maxContent))
			sb.WriteString("\n")
		}
	}
	for _, part := range m.AssistantGenMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			sb.WriteString(truncateContent(part.Text, maxContent))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// truncateContent 按 token 估算截断字符串，保留首尾、截断中间。maxTokens <= 0 时不截断
func truncateContent(s string, maxTokens int) string {
	if maxTokens <= 0 || s == "" {
		return s
	}
	tokens := countTokensInString(s)
	if tokens <= int64(maxTokens) {
		return s
	}
	// 首尾各保留一半
	halfTokens := maxTokens / 2
	headRatio := float64(halfTokens) / float64(tokens)
	headCutoff := int(float64(len(s)) * headRatio)
	// 确保不在 UTF-8 多字节序列中间截断
	for headCutoff > 0 && headCutoff < len(s) && s[headCutoff]&0xC0 == 0x80 {
		headCutoff--
	}
	tailRatio := float64(halfTokens) / float64(tokens)
	tailStart := len(s) - int(float64(len(s))*tailRatio)
	for tailStart < len(s) && s[tailStart]&0xC0 == 0x80 {
		tailStart++
	}
	if headCutoff >= tailStart {
		return s
	}
	return s[:headCutoff] + "\n...[truncated]...\n" + s[tailStart:]
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

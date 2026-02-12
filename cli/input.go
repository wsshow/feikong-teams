package cli

import (
	"regexp"
	"strings"
)

// InputBuffer 输入缓冲区，处理多行输入
type InputBuffer struct {
	lines        []string // 存储已输入的所有行
	isContinuing bool     // 是否处于续行状态
}

// NewInputBuffer 创建输入缓冲区
func NewInputBuffer() *InputBuffer {
	return &InputBuffer{
		lines:        make([]string, 0),
		isContinuing: false,
	}
}

// HandleInput 处理输入，返回完整命令或空字符串（如果需要续行）
func (b *InputBuffer) HandleInput(in string) (finalCmd string, needContinue bool) {
	cleanIn := strings.TrimSpace(in)
	// 如果以 \ 结尾，表示要续行
	if before, ok := strings.CutSuffix(cleanIn, "\\"); ok {
		b.lines = append(b.lines, before)
		b.isContinuing = true
		return "", true
	}
	// 否则，合并所有行并返回
	b.lines = append(b.lines, cleanIn)
	finalCmd = strings.Join(b.lines, "\n")
	// 执行完毕，重置状态
	b.lines = []string{}
	b.isContinuing = false
	return finalCmd, false
}

// IsContinuing 是否处于续行状态
func (b *InputBuffer) IsContinuing() bool {
	return b.isContinuing
}

// Reset 重置缓冲区
func (b *InputBuffer) Reset() {
	b.lines = []string{}
	b.isContinuing = false
}

// ExtractAgentMention 提取输入中的智能体 @ 提及
// 返回智能体名字（不含@符号）和剩余的查询内容
func ExtractAgentMention(input string) (agentName string, query string) {
	input = strings.TrimSpace(input)
	// 匹配 @智能体名称 模式（名字可以是中文或英文）
	re := regexp.MustCompile(`^@([\p{Han}\w]+)\s*(.*)$`)
	matches := re.FindStringSubmatch(input)
	if len(matches) == 3 {
		return matches[1], strings.TrimSpace(matches[2])
	}
	return "", input
}

// WorkMode 工作模式
type WorkMode string

const (
	ModeTeam   WorkMode = "team"
	ModeDeep   WorkMode = "deep"
	ModeGroup  WorkMode = "group"
	ModeCustom WorkMode = "custom"
)

// String 返回模式字符串
func (m WorkMode) String() string {
	return string(m)
}

// GetPromptPrefix 获取提示符前缀
func (m WorkMode) GetPromptPrefix() string {
	switch m {
	case ModeTeam:
		return "团队模式> "
	case ModeDeep:
		return "深度模式> "
	case ModeGroup:
		return "多智能体讨论模式> "
	case ModeCustom:
		return "自定义会议模式> "
	default:
		return "未知模式> "
	}
}

// ParseWorkMode 解析工作模式
func ParseWorkMode(mode string) WorkMode {
	switch mode {
	case "team":
		return ModeTeam
	case "deep":
		return ModeDeep
	case "group":
		return ModeGroup
	case "custom":
		return ModeCustom
	default:
		return ModeTeam
	}
}

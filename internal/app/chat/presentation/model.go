// Package presentation 将运行事件投影为客户端直接渲染的有序会话。
package presentation

import (
	"fkteams/internal/domain/message"
	"time"
)

// Snapshot 是历史、实时快照和分享共用的展示契约。
type Snapshot struct {
	Items []Item `json:"items"`
}

// Update 只更新发生变化的根节点；追加正文不重复发送已有内容。
type Update struct {
	Reset   *Snapshot `json:"reset,omitempty"`
	Remove  []string  `json:"remove,omitempty"`
	Items   []Item    `json:"items,omitempty"`
	Appends []Append  `json:"appends,omitempty"`
}

// Append 向指定根节点下的稳定分段追加文本。
type Append struct {
	MessageID string `json:"message_id"`
	Type      string `json:"type"`
	ItemID    string `json:"item_id"`
	Key       string `json:"key"`
	Content   string `json:"content"`
}

// Item 是一个有明确身份和顺序的展示节点。
type Item struct {
	Index    int       `json:"index"`
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Order    int64     `json:"order"`
	Node     *Node     `json:"node,omitempty"`
	Member   *Member   `json:"member,omitempty"`
	Ask      *Ask      `json:"ask,omitempty"`
	Approval *Approval `json:"approval,omitempty"`
	Error    *Error    `json:"error,omitempty"`
}

// Node 由后端确定消息内容、分段与操作状态。
type Node struct {
	Message        ViewMessage `json:"message"`
	Parts          []Part      `json:"parts"`
	ShowAgentLabel bool        `json:"showAgentLabel"`
	ShowCopyAction bool        `json:"showCopyAction"`
	Order          int64       `json:"order"`
}

// ViewMessage 只包含展示所需信息，不向客户端泄露运行载荷。
type ViewMessage struct {
	ID           string                `json:"id"`
	Role         message.Role          `json:"role"`
	Agent        string                `json:"agent,omitempty"`
	Content      string                `json:"content"`
	ContentParts []message.ContentPart `json:"contentParts,omitempty"`
	CreatedAt    time.Time             `json:"createdAt"`
	RunID        string                `json:"run_id,omitempty"`
}

// Part 是具有稳定身份的正文、思考、工具或交互分段。
type Part struct {
	Type      string  `json:"type"`
	Key       string  `json:"key"`
	Content   string  `json:"content,omitempty"`
	Streaming bool    `json:"streaming,omitempty"`
	Tool      *Tool   `json:"tool,omitempty"`
	Member    *Member `json:"member,omitempty"`
	Ask       *Ask    `json:"ask,omitempty"`
}

// Tool 保存后端确认的工具归属与执行状态。
type Tool struct {
	ID          string `json:"id"`
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Target      string `json:"target,omitempty"`
	Arguments   string `json:"arguments,omitempty"`
	Result      string `json:"result,omitempty"`
	Status      string `json:"status"`
	MessageID   string `json:"message_id"`
	MemberName  string `json:"member_name,omitempty"`
}

// Member 保存一次成员委派的有序内容。
type Member struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
	ToolCount int    `json:"toolCount"`
	Parts     []Part `json:"parts"`
}

// Ask 保存问答状态，客户端无需解析工具参数来恢复交互。
type Ask struct {
	Active      bool     `json:"active"`
	RunID       string   `json:"run_id,omitempty"`
	ID          string   `json:"id"`
	SessionID   string   `json:"sessionID,omitempty"`
	MessageID   string   `json:"messageID,omitempty"`
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	MultiSelect bool     `json:"multiSelect"`
	Selected    []string `json:"selected"`
	FreeText    string   `json:"freeText"`
	Answered    bool     `json:"answered"`
	MemberID    string   `json:"memberID,omitempty"`
}

// Approval 保存审批请求及其明确结果。
type Approval struct {
	Active   bool   `json:"active"`
	RunID    string `json:"run_id,omitempty"`
	ID       string `json:"id"`
	Order    int64  `json:"order"`
	Message  string `json:"message"`
	Answered bool   `json:"answered"`
	Decision string `json:"decision,omitempty"`
}

// Error 保存经过规范化的可展示错误。
type Error struct {
	ID              string   `json:"id"`
	Order           int64    `json:"order"`
	Title           string   `json:"title,omitempty"`
	Message         string   `json:"message"`
	Suggestions     []string `json:"suggestions,omitempty"`
	TechnicalDetail string   `json:"technicalDetail,omitempty"`
}

// Clone 返回隔离的展示快照。
func (s Snapshot) Clone() Snapshot {
	result := Snapshot{Items: make([]Item, len(s.Items))}
	for i, item := range s.Items {
		if item.Node != nil {
			node := *item.Node
			node.Message.ContentParts = (message.Message{ContentParts: node.Message.ContentParts}).Clone().ContentParts
			node.Parts = cloneParts(node.Parts)
			item.Node = &node
		}
		if item.Member != nil {
			item.Member = cloneMember(item.Member)
		}
		if item.Ask != nil {
			item.Ask = cloneAsk(item.Ask)
		}
		if item.Approval != nil {
			approval := *item.Approval
			item.Approval = &approval
		}
		if item.Error != nil {
			failure := *item.Error
			failure.Suggestions = append([]string(nil), failure.Suggestions...)
			item.Error = &failure
		}
		result.Items[i] = item
	}
	return result
}
func cloneParts(parts []Part) []Part {
	if parts == nil {
		return nil
	}
	result := append([]Part{}, parts...)
	for i := range result {
		part := &result[i]
		if part.Tool != nil {
			tool := *part.Tool
			part.Tool = &tool
		}
		if part.Member != nil {
			part.Member = cloneMember(part.Member)
		}
		if part.Ask != nil {
			part.Ask = cloneAsk(part.Ask)
		}
	}
	return result
}
func cloneMember(source *Member) *Member {
	member := *source
	member.Parts = cloneParts(member.Parts)
	return &member
}
func cloneAsk(source *Ask) *Ask {
	ask := *source
	ask.Options = append([]string{}, ask.Options...)
	ask.Selected = append([]string{}, ask.Selected...)
	return &ask
}

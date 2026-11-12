package session

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"fkteams/internal/domain/apperror"
	domainhistory "fkteams/internal/domain/history"
	domainsession "fkteams/internal/domain/session"
	storageport "fkteams/internal/ports/storage"
)

// QueryService 提供跨会话的只读检索，不向模型暴露存储路径和工具原始载荷。
type QueryService struct {
	repository storageport.SessionRepository
	reader     storageport.SessionMessageReader
}

// NewQueryService 使用当前应用的会话存储创建查询用例。
func NewQueryService(repository storageport.SessionRepository, reader storageport.SessionMessageReader) *QueryService {
	return &QueryService{repository: repository, reader: reader}
}

// QueryRequest 按最近更新顺序分页扫描会话；Query 为空时只列出元数据。
type QueryRequest struct {
	Query  string `json:"query,omitempty" jsonschema:"description=标题或消息正文中的关键词；留空列出所有会话"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=会话扫描起点，首次为 0，后续使用 next_offset"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=本次扫描的会话数量，默认 20，最大 50"`
}

// SessionMatch 包含会话身份和命中片段。
type SessionMatch struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageIndex *int      `json:"message_index,omitempty"`
	Snippet      string    `json:"snippet,omitempty"`
}

// QueryResult 显式返回扫描进度，空匹配页不代表已搜索所有会话。
type QueryResult struct {
	Sessions      []SessionMatch `json:"sessions"`
	TotalSessions int            `json:"total_sessions"`
	NextOffset    int            `json:"next_offset"`
	HasMore       bool           `json:"has_more"`
}

// ReadRequest 以消息索引分页读取正文；长消息可通过字符偏移继续读取。
type ReadRequest struct {
	SessionID     string `json:"session_id" jsonschema:"description=会话列表或搜索结果中的 session_id,required"`
	Offset        int    `json:"offset,omitempty" jsonschema:"description=消息索引，首次为 0"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=读取消息数量，默认 10，最大 20"`
	ContentOffset int    `json:"content_offset,omitempty" jsonschema:"description=第一条消息的字符偏移，用于续读被截断的长消息"`
}

// MessageText 是供检索使用的正文投影，不包含思考、工具参数或附件数据。
type MessageText struct {
	Index             int       `json:"index"`
	Agent             string    `json:"agent"`
	CreatedAt         time.Time `json:"created_at"`
	Content           string    `json:"content"`
	NextContentOffset int       `json:"next_content_offset"`
	Truncated         bool      `json:"truncated"`
}

// ReadResult 返回消息页及继续读取所需的游标。
type ReadResult struct {
	SessionID  string        `json:"session_id"`
	Title      string        `json:"title"`
	Messages   []MessageText `json:"messages"`
	Total      int           `json:"total"`
	NextOffset int           `json:"next_offset"`
	HasMore    bool          `json:"has_more"`
}

// Query 扫描指定会话页，在标题和消息正文中匹配大小写无关的关键词。
func (s *QueryService) Query(ctx context.Context, req QueryRequest) (QueryResult, error) {
	if req.Offset < 0 || req.Limit < 0 {
		return QueryResult{}, invalidQuery("offset and limit must be non-negative")
	}
	if s == nil || s.repository == nil || s.reader == nil {
		return QueryResult{}, invalidQuery("session query is not configured")
	}
	records, err := s.repository.ListSessions(ctx)
	if err != nil {
		return QueryResult{}, err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Metadata.UpdatedAt.Equal(records[j].Metadata.UpdatedAt) {
			return records[i].Metadata.ID < records[j].Metadata.ID
		}
		return records[i].Metadata.UpdatedAt.After(records[j].Metadata.UpdatedAt)
	})
	start := min(req.Offset, len(records))
	end := start + min(pageLimit(req.Limit, 20, 50), len(records)-start)
	result := QueryResult{Sessions: []SessionMatch{}, TotalSessions: len(records), NextOffset: end, HasMore: end < len(records)}
	keyword := strings.ToLower(strings.TrimSpace(req.Query))
	if len([]rune(keyword)) > 200 {
		return QueryResult{}, invalidQuery("query must not exceed 200 characters")
	}
	for _, record := range records[start:end] {
		if err := ctx.Err(); err != nil {
			return QueryResult{}, err
		}
		meta := record.Metadata
		match := SessionMatch{SessionID: meta.ID, Title: meta.Title, UpdatedAt: meta.UpdatedAt}
		if keyword == "" || strings.Contains(strings.ToLower(meta.Title), keyword) {
			result.Sessions = append(result.Sessions, match)
			continue
		}
		messages, err := s.messages(ctx, meta.ID)
		if err != nil {
			return QueryResult{}, err
		}
		for i, message := range messages {
			text := message.GetTextContent()
			lower := strings.ToLower(text)
			if at := strings.Index(lower, keyword); at >= 0 {
				// 按字符裁切，中文命中附近的片段不会破坏 UTF-8。
				index := len([]rune(lower[:at]))
				runes := []rune(text)
				from := max(0, index-80)
				match.MessageIndex = &i
				match.Snippet = string(runes[from:min(len(runes), from+320)])
				result.Sessions = append(result.Sessions, match)
				break
			}
		}
	}
	return result, nil
}

// Read 校验会话身份后读取指定范围的正文。
func (s *QueryService) Read(ctx context.Context, req ReadRequest) (ReadResult, error) {
	if !domainsession.ValidID(req.SessionID) {
		return ReadResult{}, invalidQuery("invalid session ID")
	}
	if req.Offset < 0 || req.Limit < 0 || req.ContentOffset < 0 {
		return ReadResult{}, invalidQuery("offsets and limit must be non-negative")
	}
	if s == nil || s.repository == nil || s.reader == nil {
		return ReadResult{}, invalidQuery("session query is not configured")
	}
	if err := ctx.Err(); err != nil {
		return ReadResult{}, err
	}
	meta, err := s.repository.LoadSession(ctx, req.SessionID)
	if err != nil {
		return ReadResult{}, err
	}
	messages, err := s.messages(ctx, req.SessionID)
	if err != nil {
		return ReadResult{}, err
	}
	start := min(req.Offset, len(messages))
	end := start + min(pageLimit(req.Limit, 10, 20), len(messages)-start)
	result := ReadResult{SessionID: meta.ID, Title: meta.Title, Messages: []MessageText{}, Total: len(messages), NextOffset: end, HasMore: end < len(messages)}
	for i := start; i < end; i++ {
		msg := messages[i]
		content := []rune(msg.GetTextContent())
		from := 0
		if i == start {
			from = min(req.ContentOffset, len(content))
		}
		to := from + min(4000, len(content)-from)
		result.Messages = append(result.Messages, MessageText{Index: i, Agent: msg.AgentName, CreatedAt: msg.StartTime, Content: string(content[from:to]), NextContentOffset: to, Truncated: to < len(content)})
	}
	return result, nil
}

func (s *QueryService) messages(ctx context.Context, id string) ([]domainhistory.AgentMessage, error) {
	messages, err := s.reader.LoadSessionMessages(ctx, id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return messages, err
}

func invalidQuery(message string) error { return apperror.New(apperror.CodeInvalidArgument, message) }
func pageLimit(limit, fallback, maximum int) int {
	if limit == 0 {
		return fallback
	}
	return min(limit, maximum)
}

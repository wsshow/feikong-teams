package handler

import (
	"errors"
	"fkteams/internal/adapters/storage/file/history"
	sharestore "fkteams/internal/adapters/storage/file/share"
	domainmessage "fkteams/internal/domain/message"
	storageport "fkteams/internal/ports/storage"
	"fkteams/internal/runtime/log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// SessionShare 会话分享列表展示信息
type SessionShare struct {
	ID               string `json:"id"`
	SessionID        string `json:"session_id"`
	Title            string `json:"title"`
	HasPassword      bool   `json:"has_password"`
	AllowToolDetails bool   `json:"allow_tool_details"`
	MessageCount     int    `json:"message_count"`
	ExpiresAt        int64  `json:"expires_at"`
	CreatedAt        int64  `json:"created_at"`
	LastAccessedAt   int64  `json:"last_accessed_at,omitempty"`
}

func sessionShareResponse(id string, entry *sharestore.SessionEntry) SessionShare {
	if entry == nil {
		return SessionShare{ID: id}
	}
	return SessionShare{
		ID:               id,
		SessionID:        entry.SessionID,
		Title:            entry.Title,
		HasPassword:      entry.PasswordHash != "",
		AllowToolDetails: entry.AllowToolDetails,
		MessageCount:     entry.MessageCount,
		ExpiresAt:        sharestore.ExpiresAtUnix(entry.ExpiresAt),
		CreatedAt:        entry.CreatedAt.Unix(),
		LastAccessedAt:   sharestore.ExpiresAtUnix(entry.LastAccessedAt),
	}
}

func sessionShareMessages(historyDir, sessionID string, allowToolDetails bool, stores ...storageport.Documents) ([]eventlog.AgentMessage, error) {
	if !validateSessionID(sessionID) {
		return nil, errors.New("invalid session ID")
	}
	recorder := eventlog.NewHistoryRecorder()
	if len(stores) > 0 {
		recorder.Documents = stores[0]
	}
	sessionDir := sessionDirPath(historyDir, sessionID)
	recorder.SetSessionDir(sessionDir)
	transcriptFile := filepath.Join(sessionDir, eventlog.TranscriptFileName)
	if err := recorder.LoadFromFile(transcriptFile); err != nil {
		return nil, err
	}
	messages := recorder.GetMessages()
	if allowToolDetails {
		return messages, nil
	}
	for msgIndex := range messages {
		events := messages[msgIndex].Events
		for eventIndex := range events {
			events[eventIndex].Message = nil
			for i := range events[eventIndex].ContentParts {
				events[eventIndex].ContentParts[i].Extra = nil
			}
			if events[eventIndex].ToolCall != nil {
				events[eventIndex].ToolCall.Arguments = ""
				events[eventIndex].ToolCall.Result = ""
				events[eventIndex].ToolCall.Extra = nil
			}
			events[eventIndex].Detail = ""
		}
		messages[msgIndex].Events = events
	}
	return messages, nil
}

func sessionShareTranscript(historyDir, sessionID string, allowToolDetails bool, stores ...storageport.Documents) ([]eventlog.TranscriptEvent, error) {
	if !validateSessionID(sessionID) {
		return nil, errors.New("invalid session ID")
	}
	records, err := eventlog.LoadSessionTranscriptRecords(sessionDirPath(historyDir, sessionID), stores...)
	lines := make([]eventlog.TranscriptEvent, len(records))
	for i, record := range records {
		lines[i] = record.Event
	}
	if err != nil {
		return nil, err
	}
	if allowToolDetails {
		return lines, nil
	}
	for index := range lines {
		if envelope := lines[index].Envelope; envelope != nil {
			envelope.ToolArgs, envelope.ToolResult, envelope.Content, envelope.Error = "", "", "", ""
			envelope.Message, envelope.ToolCall, envelope.ToolCalls = nil, nil, nil
			if envelope.Approval != nil {
				copy := *envelope.Approval
				copy.Message = "需要审批"
				envelope.Approval = &copy
			}
		}
		lines[index].Args = ""
		lines[index].Result = ""
		lines[index].ResultRef = ""
		lines[index].Summary = ""
		lines[index].Extra = nil
		lines[index].Detail = ""
		if source := lines[index].Message; source != nil {
			source.Extra = nil
			for i := range source.ToolCalls {
				source.ToolCalls[i].Function.Arguments = ""
				source.ToolCalls[i].Extra = nil
			}
			for i := range source.ContentParts {
				source.ContentParts[i].Extra = nil
			}
			if source.Role == domainmessage.RoleTool {
				source.Content, source.ReasoningContent = "", ""
				source.ContentParts = nil
			}
			lines[index] = lines[index].Normalize()
		}
	}
	return lines, nil
}

// CreateSessionShareHandler 创建会话分享

func (rt *Runtime) CreateSessionShareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			SessionID        string `json:"session_id" binding:"required"`
			Password         string `json:"password"`
			ExpiresIn        int64  `json:"expires_in"`
			AllowToolDetails bool   `json:"allow_tool_details"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validateSessionID(req.SessionID) {
			Fail(c, http.StatusBadRequest, "invalid session ID")
			return
		}

		sessionDir := rt.sessionDirPath(req.SessionID)
		meta, metaErr := eventlog.LoadMetadata(sessionDir, rt.Documents)
		messages, err := sessionShareMessages(rt.HistoryDir, req.SessionID, req.AllowToolDetails, rt.Documents)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || metaErr != nil {
				Fail(c, http.StatusNotFound, "session history not found")
				return
			}
			log.Printf("failed to load session share history: session=%s, err=%v", req.SessionID, err)
			Fail(c, http.StatusInternalServerError, "failed to read session history")
			return
		}
		if len(messages) == 0 {
			Fail(c, http.StatusBadRequest, "session has no shareable messages")
			return
		}

		expiresIn := req.ExpiresIn
		if expiresIn == 0 {
			expiresIn = 7 * 24 * 3600
		}
		const maxSessionShareExpiry = 90 * 24 * 3600
		if expiresIn > 0 && expiresIn > maxSessionShareExpiry {
			expiresIn = maxSessionShareExpiry
		}

		linkID, err := generateLinkID()
		if err != nil {
			Fail(c, http.StatusInternalServerError, "failed to create share")
			return
		}

		now := time.Now()
		var expiresAt time.Time
		if expiresIn >= 0 {
			expiresAt = now.Add(time.Duration(expiresIn) * time.Second)
		}
		title := req.SessionID
		if metaErr == nil && meta.Title != "" {
			title = meta.Title
		}
		entry := &sharestore.SessionEntry{
			SessionID:        req.SessionID,
			Title:            title,
			AllowToolDetails: req.AllowToolDetails,
			MessageCount:     len(messages),
			ExpiresAt:        expiresAt,
			CreatedAt:        now,
		}
		if req.Password != "" {
			entry.PasswordHash = hashPassword(req.Password)
			if entry.PasswordHash == "" {
				Fail(c, http.StatusInternalServerError, "failed to process password")
				return
			}
		}

		store := rt.SessionShares
		if err := store.Put(linkID, entry); err != nil {
			log.Printf("failed to persist session share: id=%s, err=%v", linkID, err)
			Fail(c, http.StatusInternalServerError, "failed to save session share")
			return
		}

		OK(c, sessionShareResponse(linkID, entry))
	}
}

// ListSessionSharesHandler 列出会话分享

func (rt *Runtime) ListSessionSharesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		expired := make([]string, 0)
		shares := make([]SessionShare, 0)
		store := rt.SessionShares

		for id, entry := range store.Snapshot() {
			if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
				expired = append(expired, id)
				continue
			}
			shares = append(shares, sessionShareResponse(id, entry))
		}

		if len(expired) > 0 {
			if err := store.DeleteMany(expired); err != nil {
				log.Printf("failed to persist expired session share cleanup: err=%v", err)
			}
		}

		sort.Slice(shares, func(i, j int) bool {
			return shares[i].CreatedAt > shares[j].CreatedAt
		})
		OK(c, shares)
	}
}

// DeleteSessionShareHandler 删除会话分享

func (rt *Runtime) DeleteSessionShareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := c.Param("shareID")
		if shareID == "" {
			Fail(c, http.StatusBadRequest, "missing share ID")
			return
		}

		store := rt.SessionShares
		exists, err := store.Delete(shareID)
		if !exists {
			Fail(c, http.StatusNotFound, "share not found")
			return
		}
		if err != nil {
			log.Printf("failed to persist session share deletion: id=%s, err=%v", shareID, err)
			Fail(c, http.StatusInternalServerError, "failed to delete session share")
			return
		}
		OK(c, gin.H{"message": "share deleted"})
	}
}

// GetPublicSessionShareInfoHandler 返回公开分享基础信息

func (rt *Runtime) GetPublicSessionShareInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := c.Param("shareID")
		entry, exists, expired := rt.SessionShares.EntryByID(shareID)
		if expired {
			Fail(c, http.StatusGone, "share expired")
			return
		}
		if !exists {
			Fail(c, http.StatusNotFound, "share not found")
			return
		}
		OK(c, gin.H{
			"id":                 shareID,
			"title":              entry.Title,
			"has_password":       entry.PasswordHash != "",
			"message_count":      entry.MessageCount,
			"expires_at":         sharestore.ExpiresAtUnix(entry.ExpiresAt),
			"created_at":         entry.CreatedAt.Unix(),
			"allow_tool_details": entry.AllowToolDetails,
		})
	}
}

// AccessPublicSessionShareHandler 访问公开分享内容

func (rt *Runtime) AccessPublicSessionShareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := c.Param("shareID")
		store := rt.SessionShares
		entry, exists, expired := store.EntryByID(shareID)
		if expired {
			Fail(c, http.StatusGone, "share expired")
			return
		}
		if !exists {
			Fail(c, http.StatusNotFound, "share not found")
			return
		}

		var req struct {
			Password string `json:"password"`
		}
		if c.Request.Body != nil {
			_ = c.ShouldBindJSON(&req)
		}
		if entry.PasswordHash != "" {
			if req.Password == "" {
				c.JSON(http.StatusUnauthorized, Response{
					Code:    1,
					Message: "password required",
					Data:    gin.H{"require_password": true},
				})
				return
			}
			attemptKey := "session-share:" + shareID + ":" + c.ClientIP()
			if allowed, retryAfter := publicShareAttempts.Allow(attemptKey, time.Now()); !allowed {
				rateLimitExceeded(c, retryAfter)
				return
			}
			if !verifyPassword(req.Password, entry.PasswordHash) {
				c.JSON(http.StatusUnauthorized, Response{
					Code:    1,
					Message: "invalid password",
					Data:    gin.H{"require_password": true},
				})
				return
			}
			publicShareAttempts.Reset(attemptKey)
		}

		transcript, err := sessionShareTranscript(rt.HistoryDir, entry.SessionID, entry.AllowToolDetails, rt.Documents)
		if err != nil {
			log.Printf("failed to load public session share: share=%s, session=%s, err=%v", shareID, entry.SessionID, err)
			Fail(c, http.StatusGone, "shared session unavailable")
			return
		}

		records := make([]eventlog.SessionTranscriptRecord, len(transcript))
		for i, item := range transcript {
			records[i].Event = item
		}
		projector, projectionErr := rt.transcriptPresentation(entry.SessionID, records)
		if projectionErr != nil {
			FailError(c, projectionErr)
			return
		}
		if err := store.Touch(shareID, time.Now()); err != nil {
			log.Printf("failed to persist session share access time: id=%s, err=%v", shareID, err)
		}

		OK(c, gin.H{
			"id":                 shareID,
			"title":              entry.Title,
			"view":               projector.Snapshot(),
			"message_count":      entry.MessageCount,
			"expires_at":         sharestore.ExpiresAtUnix(entry.ExpiresAt),
			"created_at":         entry.CreatedAt.Unix(),
			"allow_tool_details": entry.AllowToolDetails,
		})
	}
}

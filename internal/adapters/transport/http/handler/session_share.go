package handler

import (
	"encoding/json"
	"errors"
	"fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appdata"
	"fkteams/internal/runtime/atomicfile"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionShareFileName = "session_share.json"

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

type sessionShareEntry struct {
	SessionID        string    `json:"session_id"`
	Title            string    `json:"title"`
	PasswordHash     string    `json:"password_hash,omitempty"`
	AllowToolDetails bool      `json:"allow_tool_details"`
	MessageCount     int       `json:"message_count"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	LastAccessedAt   time.Time `json:"last_accessed_at,omitempty"`
}

type sessionShareFileEntry struct {
	SessionID        string `json:"session_id"`
	Title            string `json:"title"`
	PasswordHash     string `json:"password_hash,omitempty"`
	AllowToolDetails bool   `json:"allow_tool_details"`
	MessageCount     int    `json:"message_count"`
	ExpiresAt        int64  `json:"expires_at"`
	CreatedAt        int64  `json:"created_at"`
	LastAccessedAt   int64  `json:"last_accessed_at,omitempty"`
}

var sessionShareStore = struct {
	sync.RWMutex
	m map[string]*sessionShareEntry
}{m: make(map[string]*sessionShareEntry)}

func init() {
	loadSessionShares()
}

func sessionSharesFilePath() string {
	return filepath.Join(appdata.ShareDir(), sessionShareFileName)
}

func loadSessionShares() {
	entries, err := readSessionShareEntries(sessionSharesFilePath())
	if err != nil {
		return
	}
	sessionShareStore.Lock()
	now := time.Now()
	for id, e := range entries {
		var expiresAt time.Time
		if e.ExpiresAt > 0 {
			expiresAt = time.Unix(e.ExpiresAt, 0)
			if now.After(expiresAt) {
				continue
			}
		}
		var lastAccessedAt time.Time
		if e.LastAccessedAt > 0 {
			lastAccessedAt = time.Unix(e.LastAccessedAt, 0)
		}
		sessionShareStore.m[id] = &sessionShareEntry{
			SessionID:        e.SessionID,
			Title:            e.Title,
			PasswordHash:     e.PasswordHash,
			AllowToolDetails: e.AllowToolDetails,
			MessageCount:     e.MessageCount,
			ExpiresAt:        expiresAt,
			CreatedAt:        time.Unix(e.CreatedAt, 0),
			LastAccessedAt:   lastAccessedAt,
		}
	}
	sessionShareStore.Unlock()
}

func readSessionShareEntries(filePath string) (map[string]*sessionShareFileEntry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var entries map[string]*sessionShareFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func saveSessionShares() {
	if err := saveSessionSharesTo(sessionSharesFilePath()); err != nil {
		log.Printf("failed to save session shares: %v", err)
	}
}

func saveSessionSharesTo(filePath string) error {
	sessionShareStore.RLock()
	entries := make(map[string]*sessionShareFileEntry, len(sessionShareStore.m))
	for id, e := range sessionShareStore.m {
		entries[id] = &sessionShareFileEntry{
			SessionID:        e.SessionID,
			Title:            e.Title,
			PasswordHash:     e.PasswordHash,
			AllowToolDetails: e.AllowToolDetails,
			MessageCount:     e.MessageCount,
			ExpiresAt:        expiresAtUnix(e.ExpiresAt),
			CreatedAt:        e.CreatedAt.Unix(),
			LastAccessedAt:   expiresAtUnix(e.LastAccessedAt),
		}
	}
	sessionShareStore.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session shares: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("create share dir: %w", err)
	}
	if err := atomicfile.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write session shares: %w", err)
	}
	return nil
}

func sessionShareResponse(id string, entry *sessionShareEntry) SessionShare {
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
		ExpiresAt:        expiresAtUnix(entry.ExpiresAt),
		CreatedAt:        entry.CreatedAt.Unix(),
		LastAccessedAt:   expiresAtUnix(entry.LastAccessedAt),
	}
}

func sessionShareMessages(historyDir, sessionID string, allowToolDetails bool) ([]eventlog.AgentMessage, error) {
	if !validateSessionID(sessionID) {
		return nil, errors.New("invalid session ID")
	}
	recorder := eventlog.NewHistoryRecorder()
	histFile := filepath.Join(sessionDirPath(historyDir, sessionID), eventlog.HistoryFileName)
	if err := recorder.LoadFromFile(histFile); err != nil {
		return nil, err
	}
	messages := recorder.GetMessages()
	if allowToolDetails {
		return messages, nil
	}
	for msgIndex := range messages {
		events := messages[msgIndex].Events
		for eventIndex := range events {
			if events[eventIndex].ToolCall != nil {
				events[eventIndex].ToolCall.Arguments = ""
				events[eventIndex].ToolCall.Result = ""
			}
			if events[eventIndex].Action != nil {
				events[eventIndex].Action.Detail = ""
			}
		}
		messages[msgIndex].Events = events
	}
	return messages, nil
}

func sessionShareEntryByID(id string) (*sessionShareEntry, bool, bool) {
	sessionShareStore.RLock()
	entry, exists := sessionShareStore.m[id]
	sessionShareStore.RUnlock()
	if !exists {
		return nil, false, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		sessionShareStore.Lock()
		delete(sessionShareStore.m, id)
		sessionShareStore.Unlock()
		saveSessionShares()
		return nil, false, true
	}
	return entry, true, false
}

// CreateSessionShareHandler 创建会话分享
func CreateSessionShareHandler() gin.HandlerFunc {
	return NewRuntime().CreateSessionShareHandler()
}

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
		meta, metaErr := eventlog.LoadMetadata(sessionDir)
		messages, err := sessionShareMessages(rt.HistoryDir, req.SessionID, req.AllowToolDetails)
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
		entry := &sessionShareEntry{
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

		sessionShareStore.Lock()
		sessionShareStore.m[linkID] = entry
		sessionShareStore.Unlock()
		saveSessionShares()

		OK(c, sessionShareResponse(linkID, entry))
	}
}

// ListSessionSharesHandler 列出会话分享
func ListSessionSharesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		expired := make([]string, 0)
		shares := make([]SessionShare, 0)

		sessionShareStore.RLock()
		for id, entry := range sessionShareStore.m {
			if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
				expired = append(expired, id)
				continue
			}
			shares = append(shares, sessionShareResponse(id, entry))
		}
		sessionShareStore.RUnlock()

		if len(expired) > 0 {
			sessionShareStore.Lock()
			for _, id := range expired {
				delete(sessionShareStore.m, id)
			}
			sessionShareStore.Unlock()
			saveSessionShares()
		}

		sort.Slice(shares, func(i, j int) bool {
			return shares[i].CreatedAt > shares[j].CreatedAt
		})
		OK(c, shares)
	}
}

// DeleteSessionShareHandler 删除会话分享
func DeleteSessionShareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := c.Param("shareID")
		if shareID == "" {
			Fail(c, http.StatusBadRequest, "missing share ID")
			return
		}

		sessionShareStore.Lock()
		_, exists := sessionShareStore.m[shareID]
		if exists {
			delete(sessionShareStore.m, shareID)
		}
		sessionShareStore.Unlock()
		if !exists {
			Fail(c, http.StatusNotFound, "share not found")
			return
		}
		saveSessionShares()
		OK(c, gin.H{"message": "share deleted"})
	}
}

// GetPublicSessionShareInfoHandler 返回公开分享基础信息
func GetPublicSessionShareInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := c.Param("shareID")
		entry, exists, expired := sessionShareEntryByID(shareID)
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
			"expires_at":         expiresAtUnix(entry.ExpiresAt),
			"created_at":         entry.CreatedAt.Unix(),
			"allow_tool_details": entry.AllowToolDetails,
		})
	}
}

// AccessPublicSessionShareHandler 访问公开分享内容
func AccessPublicSessionShareHandler() gin.HandlerFunc {
	return NewRuntime().AccessPublicSessionShareHandler()
}

func (rt *Runtime) AccessPublicSessionShareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := c.Param("shareID")
		entry, exists, expired := sessionShareEntryByID(shareID)
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
		if entry.PasswordHash != "" && !verifyPassword(req.Password, entry.PasswordHash) {
			c.JSON(http.StatusUnauthorized, Response{
				Code:    1,
				Message: "password required",
				Data:    gin.H{"require_password": true},
			})
			return
		}

		messages, err := sessionShareMessages(rt.HistoryDir, entry.SessionID, entry.AllowToolDetails)
		if err != nil {
			log.Printf("failed to load public session share: share=%s, session=%s, err=%v", shareID, entry.SessionID, err)
			Fail(c, http.StatusGone, "shared session unavailable")
			return
		}

		now := time.Now()
		sessionShareStore.Lock()
		if current := sessionShareStore.m[shareID]; current != nil {
			current.LastAccessedAt = now
		}
		sessionShareStore.Unlock()
		saveSessionShares()

		OK(c, gin.H{
			"id":                 shareID,
			"title":              entry.Title,
			"messages":           messages,
			"message_count":      len(messages),
			"expires_at":         expiresAtUnix(entry.ExpiresAt),
			"created_at":         entry.CreatedAt.Unix(),
			"allow_tool_details": entry.AllowToolDetails,
		})
	}
}

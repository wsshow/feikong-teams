package attachment

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/appdata"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/domain/session"
)

const maxDataURLBase64Len = 256 * 1024

type ListRequest struct{}

type AttachmentSummary struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	AgentName    string `json:"agent_name,omitempty"`
	MessageText  string `json:"message_text,omitempty"`
	MessageIndex int    `json:"message_index"`
	EventIndex   int    `json:"event_index"`
	PartIndex    int    `json:"part_index"`
	URL          string `json:"url,omitempty"`
	MIMEType     string `json:"mime_type,omitempty"`
	Detail       string `json:"detail,omitempty"`
	HasBase64    bool   `json:"has_base64,omitempty"`
	Base64Length int    `json:"base64_length,omitempty"`
	StartTime    string `json:"start_time,omitempty"`
}

type ListResponse struct {
	Attachments  []AttachmentSummary `json:"attachments,omitempty"`
	Total        int                 `json:"total"`
	ErrorMessage string              `json:"error_message,omitempty"`
}

type ReadRequest struct {
	AttachmentID   string `json:"attachment_id" jsonschema:"description=附件 ID，例如 history:000000:00:01,required"`
	IncludeDataURL bool   `json:"include_data_url,omitempty" jsonschema:"description=是否在附件较小时返回 data URL，默认 false"`
}

type ReadResponse struct {
	Attachment   *AttachmentDetail `json:"attachment,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
}

type AttachmentDetail struct {
	AttachmentSummary
	Text            string `json:"text,omitempty"`
	Base64Data      string `json:"base64_data,omitempty"`
	DataURL         string `json:"data_url,omitempty"`
	DataURLTruncate bool   `json:"data_url_truncated,omitempty"`
}

func List(ctx context.Context, _ *ListRequest) (*ListResponse, error) {
	messages, err := loadCurrentSessionMessages(ctx)
	if err != nil {
		return &ListResponse{ErrorMessage: err.Error()}, nil
	}
	refs := eventlog.ListAttachments(messages)
	attachments := make([]AttachmentSummary, 0, len(refs))
	for _, ref := range refs {
		attachments = append(attachments, summarize(ref))
	}
	return &ListResponse{Attachments: attachments, Total: len(attachments)}, nil
}

func Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	if req == nil || strings.TrimSpace(req.AttachmentID) == "" {
		return &ReadResponse{ErrorMessage: "attachment_id is required"}, nil
	}
	messages, err := loadCurrentSessionMessages(ctx)
	if err != nil {
		return &ReadResponse{ErrorMessage: err.Error()}, nil
	}
	ref, ok := eventlog.FindAttachment(messages, strings.TrimSpace(req.AttachmentID))
	if !ok {
		return &ReadResponse{ErrorMessage: "attachment not found"}, nil
	}
	detail := AttachmentDetail{AttachmentSummary: summarize(ref)}
	detail.Text = ref.Part.Text
	if req.IncludeDataURL {
		dataURL, truncated := dataURLForPart(ref.Part)
		detail.DataURL = dataURL
		detail.DataURLTruncate = truncated
		if !truncated {
			detail.Base64Data = ref.Part.Base64Data
		}
	}
	return &ReadResponse{Attachment: &detail}, nil
}

func loadCurrentSessionMessages(ctx context.Context) ([]eventlog.AgentMessage, error) {
	sessionID, ok := session.IDFromContext(ctx)
	if !ok || strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session_id is not available in context")
	}
	if recorder := eventlog.GlobalSessionManager.Get(sessionID); recorder != nil {
		return recorder.GetMessages(), nil
	}
	recorder := eventlog.NewHistoryRecorder()
	historyFile := filepath.Join(appdata.SessionsDir(), filepath.Base(sessionID), eventlog.HistoryFileName)
	if err := recorder.LoadFromFile(historyFile); err != nil {
		return nil, fmt.Errorf("read session history: %w", err)
	}
	return recorder.GetMessages(), nil
}

func summarize(ref eventlog.AttachmentRef) AttachmentSummary {
	part := ref.Part
	s := AttachmentSummary{
		ID:           ref.ID,
		Type:         string(part.Type),
		AgentName:    ref.AgentName,
		MessageText:  ref.MessageText,
		MessageIndex: ref.MessageIndex,
		EventIndex:   ref.EventIndex,
		PartIndex:    ref.PartIndex,
		URL:          part.URL,
		MIMEType:     part.MIMEType,
		Detail:       part.Detail,
		HasBase64:    part.Base64Data != "",
		Base64Length: len(part.Base64Data),
	}
	if !ref.StartTime.IsZero() {
		s.StartTime = ref.StartTime.Format("2006-01-02 15:04:05")
	}
	return s
}

func dataURLForPart(part domainmessage.ContentPart) (string, bool) {
	if part.Base64Data == "" {
		return "", false
	}
	if len(part.Base64Data) > maxDataURLBase64Len {
		return "", true
	}
	mimeType := part.MIMEType
	if mimeType == "" {
		mimeType = defaultMIMEType(part.Type)
	}
	return "data:" + mimeType + ";base64," + part.Base64Data, false
}

func defaultMIMEType(partType domainmessage.ContentPartType) string {
	switch partType {
	case domainmessage.ContentPartImageURL:
		return "image/png"
	case domainmessage.ContentPartAudioURL:
		return "audio/mpeg"
	case domainmessage.ContentPartVideoURL:
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

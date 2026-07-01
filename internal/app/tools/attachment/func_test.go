package attachment

import (
	"context"
	"strings"
	"testing"

	domainhistory "fkteams/internal/domain/history"
	domainmessage "fkteams/internal/domain/message"
	"fkteams/internal/domain/session"
)

type fakeSessionMessageReader struct {
	messages []domainhistory.AgentMessage
}

func (r fakeSessionMessageReader) LoadSessionMessages(context.Context, string) ([]domainhistory.AgentMessage, error) {
	return r.messages, nil
}

func fakeReader(t *testing.T, messages []domainhistory.AgentMessage) fakeSessionMessageReader {
	t.Helper()
	return fakeSessionMessageReader{messages: messages}
}

func TestListAndReadSessionAttachments(t *testing.T) {
	sessionID := "attachment-test-session"
	reader := fakeReader(t, []domainhistory.AgentMessage{messageWithImageAttachment()})
	ctx := session.WithID(context.Background(), sessionID)

	list, err := List(ctx, reader, &ListRequest{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if list.ErrorMessage != "" || list.Total != 1 {
		t.Fatalf("List = %#v, want one attachment", list)
	}
	if list.Attachments[0].ID != "history:000000:00:01" {
		t.Fatalf("attachment id = %q, want stable id", list.Attachments[0].ID)
	}

	read, err := Read(ctx, reader, &ReadRequest{AttachmentID: list.Attachments[0].ID, IncludeDataURL: true})
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if read.ErrorMessage != "" || read.Attachment == nil {
		t.Fatalf("Read = %#v, want attachment", read)
	}
	if read.Attachment.DataURL != "data:image/png;base64,abc123" {
		t.Fatalf("data_url = %q, want image data URL", read.Attachment.DataURL)
	}
}

func TestReadSessionAttachmentDefaultsToMetadataOnly(t *testing.T) {
	sessionID := "attachment-test-metadata-only"
	reader := fakeReader(t, []domainhistory.AgentMessage{messageWithImageAttachment()})
	ctx := session.WithID(context.Background(), sessionID)

	read, err := Read(ctx, reader, &ReadRequest{AttachmentID: "history:000000:00:01"})
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if read.Attachment == nil {
		t.Fatalf("Read = %#v, want attachment", read)
	}
	if read.Attachment.Base64Data != "" || read.Attachment.DataURL != "" {
		t.Fatalf("Read returned raw data by default: %#v", read.Attachment)
	}
	if !strings.Contains(read.Attachment.MessageText, "look") {
		t.Fatalf("message_text = %q, want source text", read.Attachment.MessageText)
	}
}

func messageWithImageAttachment() domainhistory.AgentMessage {
	return domainhistory.AgentMessage{
		Events: []domainhistory.MessageEvent{
			{
				Type:    domainhistory.MsgTypeText,
				Content: "look",
				ContentParts: []domainmessage.ContentPart{
					{Type: domainmessage.ContentPartText, Text: "look"},
					{Type: domainmessage.ContentPartImageURL, Base64Data: "abc123", MIMEType: "image/png"},
				},
			},
		},
	}
}

package attachment

import (
	"context"
	"strings"
	"testing"

	"fkteams/agentcore"
	"fkteams/common"
	eventlog "fkteams/events/log"
)

func TestListAndReadSessionAttachments(t *testing.T) {
	sessionID := "attachment-test-session"
	eventlog.GlobalSessionManager.Remove(sessionID)
	t.Cleanup(func() { eventlog.GlobalSessionManager.Remove(sessionID) })

	recorder := eventlog.GlobalSessionManager.GetOrCreate(sessionID, t.TempDir())
	recorder.RecordUserMessage(agentcore.Message{
		Role: agentcore.RoleUser,
		ContentParts: []agentcore.ContentPart{
			{Type: agentcore.ContentPartText, Text: "look"},
			{Type: agentcore.ContentPartImageURL, Base64Data: "abc123", MIMEType: "image/png"},
		},
	})
	ctx := common.WithSessionID(context.Background(), sessionID)

	list, err := List(ctx, &ListRequest{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if list.ErrorMessage != "" || list.Total != 1 {
		t.Fatalf("List = %#v, want one attachment", list)
	}
	if list.Attachments[0].ID != "history:000000:00:01" {
		t.Fatalf("attachment id = %q, want stable id", list.Attachments[0].ID)
	}

	read, err := Read(ctx, &ReadRequest{AttachmentID: list.Attachments[0].ID, IncludeDataURL: true})
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
	eventlog.GlobalSessionManager.Remove(sessionID)
	t.Cleanup(func() { eventlog.GlobalSessionManager.Remove(sessionID) })

	recorder := eventlog.GlobalSessionManager.GetOrCreate(sessionID, t.TempDir())
	recorder.RecordUserMessage(agentcore.Message{
		Role: agentcore.RoleUser,
		ContentParts: []agentcore.ContentPart{
			{Type: agentcore.ContentPartText, Text: "look"},
			{Type: agentcore.ContentPartImageURL, Base64Data: "abc123", MIMEType: "image/png"},
		},
	})
	ctx := common.WithSessionID(context.Background(), sessionID)

	read, err := Read(ctx, &ReadRequest{AttachmentID: "history:000000:00:01"})
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

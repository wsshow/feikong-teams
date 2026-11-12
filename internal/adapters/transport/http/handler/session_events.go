package handler

import (
	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/chat/presentation"
)

func (rt *Runtime) transcriptPresentation(sessionID string, records []eventlog.SessionTranscriptRecord) (*presentation.Projector, error) {
	projector := presentation.New(sessionID, rt.ToolDisplays)
	for _, record := range records {
		e, err := record.Event.RuntimeEvent()
		if err != nil {
			return nil, err
		}
		projector.Apply(e)
	}
	return projector, nil
}

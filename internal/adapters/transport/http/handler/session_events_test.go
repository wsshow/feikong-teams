package handler

import eventlog "fkteams/internal/adapters/storage/file/history"

func (rt *Runtime) transcriptToChatEvents(sessionID string, transcript []eventlog.TranscriptEvent) []map[string]any {
	records := make([]eventlog.SessionTranscriptRecord, len(transcript))
	for i, item := range transcript {
		records[i].Event = item
	}
	return rt.transcriptRecordsToChatEvents(sessionID, records)
}

func (rt *Runtime) transcriptRecordsToChatEvents(sessionID string, records []eventlog.SessionTranscriptRecord) []map[string]any {
	result := []map[string]any{}
	for _, record := range records {
		e, err := record.Event.RuntimeEvent()
		if err != nil {
			continue
		}
		payload := rt.convertEventToMap(e)
		payload["session_id"] = sessionID
		result = append(result, payload)
	}
	return result
}

package handler

import (
	"errors"
	eventlog "fkteams/internal/adapters/storage/file/history"
	"fkteams/internal/app/chat/presentation"
	"fkteams/internal/domain/apperror"
	domainhistory "fkteams/internal/domain/history"
)

func (rt *Runtime) transcriptPresentation(sessionID string, records []eventlog.SessionTranscriptRecord) (*presentation.Projector, error) {
	projector := presentation.New(sessionID, rt.ToolDisplays)
	for _, record := range records {
		e, err := record.Event.RuntimeEvent()
		if err != nil {
			if errors.Is(err, domainhistory.ErrUnsupportedTranscriptFormat) {
				return nil, apperror.Wrap(apperror.CodeConflict, "这个会话的记录格式较旧，当前版本暂时无法打开。原有记录仍保留，请新建会话继续使用。", err)
			}
			return nil, apperror.Wrap(apperror.CodeInternal, "暂时无法打开这个会话，请稍后重试。", err)
		}
		projector.Apply(e)
	}
	return projector, nil
}

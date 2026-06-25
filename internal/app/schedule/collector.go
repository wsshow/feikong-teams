package schedule

import (
	"fkteams/events/view"
	"fkteams/internal/domain/event"
)

// newMarkdownCollector 复用现有事件视图收集器生成后台任务结果。
func newMarkdownCollector() (func(event.Event) error, func() string) {
	return eventview.NewMarkdownCollector()
}

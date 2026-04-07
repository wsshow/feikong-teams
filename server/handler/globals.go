package handler

import (
	"fkteams/server/handler/taskstream"
	"time"
)

// GlobalStreams 统一的全局任务流管理器，同时服务 WebSocket 和 SSE 两种连接方式。
var GlobalStreams = newGlobalStreams()

func newGlobalStreams() *taskstream.Manager {
	m := taskstream.NewManager()
	m.StartCleanup(1 * time.Minute)
	return m
}

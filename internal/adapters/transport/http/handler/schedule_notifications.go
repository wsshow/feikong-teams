package handler

import (
	"net/http"
	"time"

	domainschedule "fkteams/internal/domain/schedule"
	"github.com/gin-gonic/gin"
)

// GetScheduleNotificationsHandler 返回待送达的任务结果。
func (rt *Runtime) GetScheduleNotificationsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rt.Scheduler == nil {
			OK(c, gin.H{"tasks": []domainschedule.Task{}})
			return
		}
		tasks, err := rt.Scheduler.PendingNotifications(c)
		if err != nil {
			FailError(c, err)
			return
		}
		OK(c, gin.H{"tasks": tasks})
	}
}

// AcknowledgeScheduleResultHandler 确认界面已展示特定运行的结果通知。
func (rt *Runtime) AcknowledgeScheduleResultHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rt.Scheduler == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}
		var req struct {
			RunAt time.Time `json:"run_at"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid result acknowledgment")
			return
		}
		if err := rt.Scheduler.AcknowledgeResult(c, c.Param("id"), req.RunAt); err != nil {
			FailError(c, err)
			return
		}
		OK(c, gin.H{"acknowledged": true})
	}
}

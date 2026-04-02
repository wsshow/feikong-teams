package handler

import (
	"fkteams/tools/scheduler"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetScheduleTasksHandler 获取定时任务列表
func GetScheduleTasksHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "调度器未初始化")
			return
		}

		statusFilter := c.Query("status")
		tasks, err := s.GetTasks(statusFilter)
		if err != nil {
			log.Printf("failed to get schedule tasks: %v", err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if tasks == nil {
			tasks = []scheduler.ScheduledTask{}
		}

		OK(c, gin.H{"tasks": tasks, "total": len(tasks)})
	}
}

// CancelScheduleTaskHandler 取消定时任务
func CancelScheduleTaskHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "任务 ID 不能为空")
			return
		}

		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "调度器未初始化")
			return
		}

		resp, err := s.ScheduleCancel(c, &scheduler.ScheduleCancelRequest{TaskID: taskID})
		if err != nil {
			log.Printf("failed to cancel schedule task: id=%s, err=%v", taskID, err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if !resp.Success {
			Fail(c, http.StatusBadRequest, resp.ErrorMessage)
			return
		}

		OK(c, gin.H{"message": resp.Message})
	}
}

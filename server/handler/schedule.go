package handler

import (
	"fkteams/tools/scheduler"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetScheduleTasksHandler returns all scheduled tasks
func GetScheduleTasksHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
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

// CancelScheduleTaskHandler cancels a scheduled task
func CancelScheduleTaskHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}

		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
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

// GetTaskResultHandler returns the latest result for a task
func GetTaskResultHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}

		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		result, err := s.ReadTaskResult(taskID)
		if err != nil {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}

		OK(c, gin.H{"task_id": taskID, "result": result})
	}
}

// GetTaskHistoryHandler returns the list of history entries for a task
func GetTaskHistoryHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}

		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		entries, err := s.ListHistoryEntries(taskID)
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if entries == nil {
			entries = []scheduler.HistoryEntry{}
		}

		OK(c, gin.H{"task_id": taskID, "history": entries, "total": len(entries)})
	}
}

// GetTaskHistoryFileHandler returns a specific history file content
func GetTaskHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		filename := c.Param("filename")
		if taskID == "" || filename == "" {
			Fail(c, http.StatusBadRequest, "task ID and filename are required")
			return
		}

		s := scheduler.Global()
		if s == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		content, err := s.ReadHistoryFile(taskID, filename)
		if err != nil {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}

		OK(c, gin.H{"task_id": taskID, "filename": filename, "content": content})
	}
}

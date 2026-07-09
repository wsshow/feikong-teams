package handler

import (
	domainschedule "fkteams/internal/domain/schedule"
	schedulerport "fkteams/internal/ports/scheduler"
	"fkteams/internal/runtime/log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type scheduleTaskRequest struct {
	Task      string `json:"task"`
	CronExpr  string `json:"cron_expr"`
	ExecuteAt string `json:"execute_at"`
}

func (r scheduleTaskRequest) toAddTaskRequest() schedulerport.AddTaskRequest {
	return schedulerport.AddTaskRequest{
		Task:      r.Task,
		CronExpr:  r.CronExpr,
		ExecuteAt: r.ExecuteAt,
	}
}

// GetScheduleTasksHandler 返回调度任务列表。
func GetScheduleTasksHandler() gin.HandlerFunc {
	return NewRuntime().GetScheduleTasksHandler()
}

// CreateScheduleTaskHandler 创建调度任务。
func CreateScheduleTaskHandler() gin.HandlerFunc {
	return NewRuntime().CreateScheduleTaskHandler()
}

func (rt *Runtime) CreateScheduleTaskHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req scheduleTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid task request: "+err.Error())
			return
		}
		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		task, err := service.AddTask(c, req.toAddTaskRequest())
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		OK(c, gin.H{"task": task})
	}
}

// UpdateScheduleTaskHandler 更新调度任务。
func UpdateScheduleTaskHandler() gin.HandlerFunc {
	return NewRuntime().UpdateScheduleTaskHandler()
}

func (rt *Runtime) UpdateScheduleTaskHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}
		var req scheduleTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Fail(c, http.StatusBadRequest, "invalid task request: "+err.Error())
			return
		}
		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		task, err := service.UpdateTask(c, taskID, req.toAddTaskRequest())
		if err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		OK(c, gin.H{"task": task})
	}
}

// DeleteScheduleTaskHandler 删除调度任务。
func DeleteScheduleTaskHandler() gin.HandlerFunc {
	return NewRuntime().DeleteScheduleTaskHandler()
}

func (rt *Runtime) DeleteScheduleTaskHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}
		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		if err := service.DeleteTask(c, taskID); err != nil {
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		OK(c, gin.H{"message": "task deleted"})
	}
}

func (rt *Runtime) GetScheduleTasksHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		statusFilter := c.Query("status")
		tasks, err := service.ListTasks(c, domainschedule.Status(statusFilter))
		if err != nil {
			log.Printf("failed to get schedule tasks: %v", err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if tasks == nil {
			tasks = []domainschedule.Task{}
		}

		OK(c, gin.H{"tasks": tasks, "total": len(tasks)})
	}
}

// CancelScheduleTaskHandler 取消调度任务。
func CancelScheduleTaskHandler() gin.HandlerFunc {
	return NewRuntime().CancelScheduleTaskHandler()
}

func (rt *Runtime) CancelScheduleTaskHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}

		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		if err := service.CancelTask(c, taskID); err != nil {
			log.Printf("failed to cancel schedule task: id=%s, err=%v", taskID, err)
			Fail(c, http.StatusBadRequest, err.Error())
			return
		}

		OK(c, gin.H{"message": "task cancelled"})
	}
}

// GetTaskResultHandler 返回任务最新结果。
func GetTaskResultHandler() gin.HandlerFunc {
	return NewRuntime().GetTaskResultHandler()
}

func (rt *Runtime) GetTaskResultHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}

		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		result, err := service.ReadTaskResult(c, taskID)
		if err != nil {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}

		OK(c, gin.H{"task_id": taskID, "result": result})
	}
}

// GetTaskHistoryHandler 返回任务历史结果列表。
func GetTaskHistoryHandler() gin.HandlerFunc {
	return NewRuntime().GetTaskHistoryHandler()
}

func (rt *Runtime) GetTaskHistoryHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		if taskID == "" {
			Fail(c, http.StatusBadRequest, "task ID is required")
			return
		}

		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		entries, err := service.ListHistoryEntries(c, taskID)
		if err != nil {
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if entries == nil {
			entries = []domainschedule.HistoryEntry{}
		}

		OK(c, gin.H{"task_id": taskID, "history": entries, "total": len(entries)})
	}
}

// GetTaskHistoryFileHandler 返回指定历史结果内容。
func GetTaskHistoryFileHandler() gin.HandlerFunc {
	return NewRuntime().GetTaskHistoryFileHandler()
}

func (rt *Runtime) GetTaskHistoryFileHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID := c.Param("id")
		filename := c.Param("filename")
		if taskID == "" || filename == "" {
			Fail(c, http.StatusBadRequest, "task ID and filename are required")
			return
		}

		service := rt.Scheduler
		if service == nil {
			Fail(c, http.StatusServiceUnavailable, "scheduler not initialized")
			return
		}

		content, err := service.ReadHistoryFile(c, taskID, filename)
		if err != nil {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}

		OK(c, gin.H{"task_id": taskID, "filename": filename, "content": content})
	}
}

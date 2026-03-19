package scheduler

import (
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 获取定时任务工具集合
func (s *Scheduler) GetTools() ([]tool.BaseTool, error) {
	if s == nil {
		return nil, fmt.Errorf("定时任务调度器未初始化")
	}

	var tools []tool.BaseTool

	scheduleAddTool, err := utils.InferTool("schedule_add",
		"创建定时任务。支持两种模式：1) cron 表达式（重复任务），如 '*/5 * * * *' 每5分钟、'0 9 * * *' 每天9点；2) execute_at 指定时间（一次性任务）。cron 表达式为标准5字段格式：分 时 日 月 周。",
		s.ScheduleAdd)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleAddTool)

	scheduleListTool, err := utils.InferTool("schedule_list",
		"列出所有定时任务，支持按状态过滤（pending/running/completed/failed/cancelled）。",
		s.ScheduleList)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleListTool)

	scheduleCancelTool, err := utils.InferTool("schedule_cancel",
		"取消指定的定时任务，只能取消状态为 pending 的任务。",
		s.ScheduleCancel)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleCancelTool)

	scheduleDeleteTool, err := utils.InferTool("schedule_delete",
		"永久删除指定的定时任务（从任务列表中移除）。不能删除正在执行中（running）的任务。",
		s.ScheduleDelete)
	if err != nil {
		return nil, err
	}
	tools = append(tools, scheduleDeleteTool)

	return tools, nil
}

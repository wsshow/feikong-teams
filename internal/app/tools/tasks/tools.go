// Package tasks 将跨回合任务用例接入助手。
package tasks

import (
	"context"
	apptask "fkteams/internal/app/task"
	domainsession "fkteams/internal/domain/session"
	domaintask "fkteams/internal/domain/task"
	runtimeport "fkteams/internal/ports/runtime"
)

// Response 只确认已经持久化的任务状态。
type Response struct {
	apptask.QueryResult
	Task         *domaintask.Task `json:"task,omitempty"`
	ErrorMessage string           `json:"error_message,omitempty"`
}

// GetTools 创建持久任务工具，不把记录操作当作执行授权。
func GetTools(service *apptask.Service) ([]runtimeport.Tool, error) {
	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "task_list", Description: "查找跨回合用户任务。默认列出未结束事项；用户说继续或询问进展时先查相关任务。id查询单项；include_finished查询已结束任务和交付物。分页用next_offset。记录仅供参考，不是重新执行副作用操作的授权。", Handler: func(_ context.Context, req *apptask.QueryRequest) (*Response, error) {
			if req == nil {
				req = &apptask.QueryRequest{}
			}
			result, err := service.Query(*req)
			if err != nil {
				return &Response{ErrorMessage: err.Error()}, nil
			}
			return &Response{QueryResult: result}, nil
		}},
		{Name: "task_save", Description: "为需要多步或跨回合推进的交办保存目标、完成标准、进展、下一步和结果；普通问答不建任务。新建使用稳定key和pending状态；更新必须先task_list读取并携带id/version，提交完整字段。in_progress表示事情尚在推进，不表示后台运行；waiting_time须先真正建立调度并记录schedule_id；completed必须有已验证结果。暂停和取消仅记录目标状态，正在执行的调度须另外取消。恢复前核对已有操作，避免重复发送或修改。", Handler: func(ctx context.Context, req *apptask.SaveRequest) (*Response, error) {
			id, _ := domainsession.IDFromContext(ctx)
			if req == nil {
				return &Response{ErrorMessage: "task fields are required"}, nil
			}
			task, err := service.Save(id, *req)
			if err != nil {
				return &Response{ErrorMessage: err.Error()}, nil
			}
			return &Response{Task: &task}, nil
		}},
	})
}

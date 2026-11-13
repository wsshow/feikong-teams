package command

import (
	"context"
	port "fkteams/internal/ports/process"
	"time"
)

// ReadRequest 通过任务ID与绝对字节游标读取合并日志。
type ReadRequest struct {
	TaskID string `json:"task_id"`
	Cursor *int64 `json:"cursor,omitempty" jsonschema:"description=绝对字节游标，默认-1读取最新尾部；0从最早可用位置读取"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=单次日志字节上限，默认8192，最大65536"`
	WaitMS int    `json:"wait_ms,omitempty" jsonschema:"description=等待终态的毫秒预算，0立即返回，最大30000；不会修改执行期限"`
}

// ReadResponse 包含进程状态与日志续读位置。
type ReadResponse struct {
	*SmartExecuteResponse
	port.Output
}

// StopRequest 仅允许停止当前会话和工作区所属任务。
type StopRequest struct {
	TaskID string `json:"task_id"`
}

// ListRequest 为无参数的任务列表请求。
type ListRequest struct{}

// Read 返回一页运行中或已完成日志，读取不会消费日志。
func (t *CommandTools) Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	if req == nil || req.WaitMS < 0 || req.WaitMS > 30000 {
		return &ReadResponse{SmartExecuteResponse: &SmartExecuteResponse{ErrorMessage: "wait_ms must be between 0 and 30000"}}, nil
	}
	if t.service == nil {
		return &ReadResponse{SmartExecuteResponse: &SmartExecuteResponse{ErrorMessage: "process service is not configured"}}, nil
	}
	cursor := int64(-1)
	if req.Cursor != nil {
		cursor = *req.Cursor
	}
	limit := req.Limit
	if limit == 0 {
		limit = 8192
	}
	r, out, err := t.service.Read(ctx, t.owner(ctx), req.TaskID, cursor, limit, time.Duration(req.WaitMS)*time.Millisecond)
	if err != nil {
		return &ReadResponse{SmartExecuteResponse: &SmartExecuteResponse{ErrorMessage: err.Error()}}, nil
	}
	return &ReadResponse{SmartExecuteResponse: response(r), Output: out}, nil
}

// Stop 等待任务完成优雅终止和日志收尾。
func (t *CommandTools) Stop(ctx context.Context, req *StopRequest) (*SmartExecuteResponse, error) {
	if req == nil {
		return &SmartExecuteResponse{ErrorMessage: "request is required"}, nil
	}
	if t.service == nil {
		return &SmartExecuteResponse{ErrorMessage: "process service is not configured"}, nil
	}
	r, err := t.service.StopTask(ctx, t.owner(ctx), req.TaskID)
	if err != nil {
		return &SmartExecuteResponse{ErrorMessage: err.Error()}, nil
	}
	resp := response(r)
	resp.Success = true
	resp.ErrorMessage = ""
	return resp, nil
}

// List 只列出当前会话和工作区的任务。
func (t *CommandTools) List(ctx context.Context, _ *ListRequest) (*SmartExecuteResponse, error) {
	if t.service == nil {
		return &SmartExecuteResponse{ErrorMessage: "process service is not configured"}, nil
	}
	records, err := t.service.List(t.owner(ctx))
	if err != nil {
		return &SmartExecuteResponse{ErrorMessage: err.Error()}, nil
	}
	resp := &SmartExecuteResponse{Success: true}
	for _, r := range records {
		resp.Tasks = append(resp.Tasks, BackgroundTaskInfo{TaskID: r.ID, Command: r.Command, Status: r.State, ElapsedTime: response(r).ExecutionTime})
	}
	return resp, nil
}

func (t *CommandTools) handleTaskOperation(ctx context.Context, req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
	switch req.TaskAction {
	case "list":
		return t.List(ctx, &ListRequest{})
	case "terminate":
		return t.Stop(ctx, &StopRequest{TaskID: req.TaskID})
	case "status", "":
		resp, err := t.Read(ctx, &ReadRequest{TaskID: req.TaskID})
		if resp != nil {
			resp.OutputPreview = resp.Output.Text
			return resp.SmartExecuteResponse, err
		}
		return nil, err
	default:
		return &SmartExecuteResponse{ErrorMessage: "invalid task operation"}, nil
	}
}

package event

import "encoding/json"

// ToolStatus 是工具执行的明确状态，空输出不代表执行失败。
type ToolStatus string

const (
	ToolPending     ToolStatus = "pending"
	ToolRunning     ToolStatus = "running"
	ToolCompleted   ToolStatus = "completed"
	ToolError       ToolStatus = "error"
	ToolCancelled   ToolStatus = "cancelled"
	ToolInterrupted ToolStatus = "interrupted"
)

// ToolResultStatus 统一响应中的业务失败标记，避免客户端或摘要回放重新判断结果。
func ToolResultStatus(result string) ToolStatus {
	var payload struct {
		Success  *bool  `json:"success"`
		Error    string `json:"error_message"`
		ExitCode int    `json:"exit_code"`
	}
	if json.Unmarshal([]byte(result), &payload) == nil && (payload.Success != nil && !*payload.Success || payload.Error != "" || payload.ExitCode != 0) {
		return ToolError
	}
	return ToolCompleted
}

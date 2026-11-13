package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appprocess "fkteams/internal/app/process"
	domain "fkteams/internal/domain/process"
	"fkteams/internal/domain/session"
	"fkteams/internal/runtime/approval"
)

// ApprovalMode 指定危险命令的审批策略。
type ApprovalMode int

const (
	ApprovalModeHITL ApprovalMode = iota
	ApprovalModeReject
)

// Option 配置命令工具依赖。
type Option func(*CommandTools)

// WithApprovalMode 设置审批模式。
func WithApprovalMode(mode ApprovalMode) Option {
	return func(t *CommandTools) { t.approvalMode = mode }
}

// WithService 注入应用实例持有的进程用例。
func WithService(service *appprocess.Service) Option {
	return func(t *CommandTools) { t.service = service }
}

// CommandTools 负责命令参数与安全审批，任务状态由应用用例持有。
type CommandTools struct {
	service      *appprocess.Service
	workDir      string
	approvalMode ApprovalMode
}

// NewCommandTools 创建工具声明；执行时必须显式注入进程用例。
func NewCommandTools(workDir string, opts ...Option) *CommandTools {
	t := &CommandTools{workDir: workDir}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// SmartExecuteRequest 将命令执行期限与本次等待预算分开。
type SmartExecuteRequest struct {
	Command    string `json:"command,omitempty" jsonschema:"description=要执行的 shell 命令"`
	Reason     string `json:"reason,omitempty" jsonschema:"description=执行目的"`
	Mode       string `json:"mode,omitempty" jsonschema:"description=job（默认）为有限任务；service 为常驻服务"`
	Timeout    int    `json:"timeout,omitempty" jsonschema:"description=执行期限秒数。0或省略使用模式默认值：job 120秒，service 无限；显式正值范围1到86400"`
	YieldMS    *int   `json:"yield_ms,omitempty" jsonschema:"description=本次等待预算毫秒，默认1000，范围0到30000。耗尽后返回任务ID，不改变执行期限"`
	Background bool   `json:"background,omitempty" jsonschema:"description=立即返回任务ID。后台运行仍受 mode 和 timeout 管理，服务请同时指定 mode=service"`
	TaskID     string `json:"task_id,omitempty" jsonschema:"description=兼容旧接口的任务ID，推荐使用 process_read 或 process_stop"`
	TaskAction string `json:"task_action,omitempty" jsonschema:"description=兼容旧接口：list、status、terminate"`
}

// SmartExecuteResponse 返回稳定任务ID、明确状态及有界输出尾部。
type SmartExecuteResponse struct {
	Success         bool                 `json:"success"`
	Stdout          string               `json:"stdout,omitempty"`
	Stderr          string               `json:"stderr,omitempty"`
	ExitCode        *int                 `json:"exit_code,omitempty"`
	ExecutionTime   string               `json:"execution_time,omitempty"`
	SecurityLevel   string               `json:"security_level,omitempty"`
	Command         string               `json:"command,omitempty"`
	ErrorMessage    string               `json:"error_message,omitempty"`
	WarningMessage  string               `json:"warning_message,omitempty"`
	OutputTruncated bool                 `json:"output_truncated,omitempty"`
	OutputPreview   string               `json:"output_preview,omitempty"`
	IsBackground    bool                 `json:"is_background,omitempty"`
	TaskID          string               `json:"task_id,omitempty"`
	PID             int                  `json:"pid,omitempty"`
	Status          domain.State         `json:"status,omitempty"`
	Tasks           []BackgroundTaskInfo `json:"tasks,omitempty"`
}

// BackgroundTaskInfo 是当前会话内可见的任务摘要。
type BackgroundTaskInfo struct {
	TaskID      string       `json:"task_id"`
	Command     string       `json:"command"`
	Status      domain.State `json:"status"`
	ElapsedTime string       `json:"elapsed_time"`
}

func (t *CommandTools) owner(ctx context.Context) domain.Owner {
	id, _ := session.IDFromContext(ctx)
	return domain.Owner{Workspace: t.workDir, SessionID: id}
}

// SmartExecute 智能执行命令，危险命令根据审批模式处理
func (t *CommandTools) SmartExecute(ctx context.Context, req *SmartExecuteRequest) (*SmartExecuteResponse, error) {
	if req == nil {
		return &SmartExecuteResponse{ErrorMessage: "request is required"}, nil
	}
	if err := ctx.Err(); err != nil {
		return &SmartExecuteResponse{ErrorMessage: err.Error()}, nil
	}
	// 后台任务管理操作
	if req.TaskAction != "" || req.TaskID != "" {
		return t.handleTaskOperation(ctx, req)
	}

	if req.Command == "" {
		return &SmartExecuteResponse{ErrorMessage: "command is required"}, nil
	}

	eval := evaluateSecurity(req.Command)

	if eval.Level == LevelDangerous {
		if t.approvalMode == ApprovalModeReject {
			return &SmartExecuteResponse{
				Command:       req.Command,
				SecurityLevel: securityLevelName(eval.Level),
				ErrorMessage: fmt.Sprintf("命令被拒绝：%s — %s",
					eval.Description, strings.Join(eval.Risks, "; ")),
			}, nil
		}

		if err := approval.RequireOperation(ctx, approval.Operation{
			StoreName: approval.StoreCommand,
			Key:       req.Command,
			Title:     "Dangerous command requires approval",
			Target:    req.Command,
			Details: []approval.OperationDetail{
				{Name: "Reason", Value: req.Reason},
				{Name: "SecurityLevel", Value: securityLevelName(eval.Level)},
				{Name: "Description", Value: eval.Description},
				{Name: "Risks", Value: strings.Join(eval.Risks, "; ")},
			},
		}); err != nil {
			if errors.Is(err, approval.ErrRejected) {
				return &SmartExecuteResponse{
					Command:       req.Command,
					SecurityLevel: securityLevelName(eval.Level),
					ErrorMessage:  "command rejected by user",
				}, nil
			}
			return nil, err
		}
	}

	return t.executeCommand(ctx, req, eval)
}

func (t *CommandTools) executeCommand(ctx context.Context, req *SmartExecuteRequest, eval SecurityEvaluation) (*SmartExecuteResponse, error) {
	if t.service == nil {
		return &SmartExecuteResponse{ErrorMessage: "process service is not configured"}, nil
	}
	mode := req.Mode
	if mode == "" {
		mode = "job"
	}
	if mode != "job" && mode != "service" {
		return &SmartExecuteResponse{ErrorMessage: "mode must be job or service"}, nil
	}
	if req.Timeout < 0 || req.Timeout > 86400 {
		return &SmartExecuteResponse{ErrorMessage: "timeout must be between 1 and 86400 seconds when specified"}, nil
	}
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout == 0 && mode == "job" {
		timeout = 120 * time.Second
	}
	yield := time.Second
	if req.YieldMS != nil {
		if *req.YieldMS < 0 || *req.YieldMS > 30000 {
			return &SmartExecuteResponse{ErrorMessage: "yield_ms must be between 0 and 30000"}, nil
		}
		yield = time.Duration(*req.YieldMS) * time.Millisecond
	}
	record, err := t.service.Execute(ctx, appprocess.Request{Owner: t.owner(ctx), Command: req.Command, Mode: domain.Mode(mode), Timeout: timeout, Yield: yield, Background: req.Background})
	if err != nil {
		return &SmartExecuteResponse{ErrorMessage: err.Error()}, nil
	}
	resp := response(record)
	resp.SecurityLevel = securityLevelName(eval.Level)
	if eval.Level == LevelModerate {
		resp.WarningMessage = fmt.Sprintf("中等风险命令: %s - %s", eval.Description, strings.Join(eval.Risks, "; "))
	}
	return resp, nil
}

func response(record domain.Record) *SmartExecuteResponse {
	elapsed := time.Since(record.StartedAt)
	if !record.FinishedAt.IsZero() {
		elapsed = record.FinishedAt.Sub(record.StartedAt)
	}
	resp := &SmartExecuteResponse{Success: record.State == domain.Running || record.State == domain.Succeeded, TaskID: record.ID, Command: record.Command, Status: record.State, PID: record.PID, Stdout: record.Stdout, Stderr: record.Stderr, ExitCode: record.ExitCode, ExecutionTime: elapsed.Round(time.Millisecond).String(), ErrorMessage: record.Error, IsBackground: record.Background, OutputTruncated: record.OutputBytes > int64(len(record.Stdout)+len(record.Stderr))}
	if record.State == domain.Failed && record.ExitCode != nil {
		if hint := interpretExitCode(record.Command, *record.ExitCode); hint != "" {
			resp.ErrorMessage = hint
		}
	}
	return resp
}

// interpretExitCode 将非零退出码映射为人类可读的解释
func interpretExitCode(command string, code int) string {
	cmd := strings.TrimSpace(strings.ToLower(command))

	matches := func(prefix string) bool { return strings.HasPrefix(cmd, prefix) }

	switch {
	case matches("grep") && code == 1:
		return "grep: 未找到匹配项（此为退出码 1，非错误）"
	case matches("diff") && code == 1:
		return "diff: 文件存在差异（此为退出码 1，非错误）"
	case matches("curl") && code >= 6 && code <= 89:
		return fmt.Sprintf("curl: %s", curlExitMeaning(code))
	case matches("git diff") && code == 1:
		return "git diff: 文件存在差异（此为退出码 1，非错误）"
	case matches("go test") && code == 1:
		return "go test: 测试失败"
	case matches("go build") && code > 0:
		return "go build: 编译失败，请检查错误输出"
	case matches("git grep") && code == 1:
		return "git grep: 未找到匹配项"
	}
	if code >= 126 && code <= 128 {
		return fmt.Sprintf("命令无法执行（退出码 %d：%s）", code, shellExitMeaning(code))
	}
	if code > 128 {
		return fmt.Sprintf("命令被信号 %d 终止", code-128)
	}
	return ""
}

func curlExitMeaning(code int) string {
	switch code {
	case 6:
		return "DNS 解析失败"
	case 7:
		return "连接被拒绝"
	case 28:
		return "连接超时"
	case 35:
		return "TLS 握手失败"
	default:
		return fmt.Sprintf("错误码 %d", code)
	}
}

func shellExitMeaning(code int) string {
	switch code {
	case 126:
		return "命令不可执行（权限问题）"
	case 127:
		return "命令未找到"
	case 128:
		return "exit 参数无效"
	}
	return ""
}

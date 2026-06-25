// Package ask 提供 askQuestions 工具，允许模型向用户提问以收集信息、观点或做出选择。
package ask

import (
	"context"
	"encoding/gob"
	runtimeport "fkteams/internal/ports/runtime"
	"fmt"

	"github.com/google/uuid"
)

type handlerContextKey struct{}

func init() {
	gob.Register(&AskInfo{})
	gob.Register(&AskResponse{})
}

// AskInfo 中断信息，传递给前端/CLI 展示问题
type AskInfo struct {
	Question    string   `json:"question"`
	Options     []string `json:"options,omitempty"`
	MultiSelect bool     `json:"multi_select,omitempty"`
}

func (a *AskInfo) String() string {
	return a.Question
}

// AskResponse 用户的回答
type AskResponse struct {
	AskID    string   `json:"ask_id,omitempty"`
	Selected []string `json:"selected,omitempty"`
	FreeText string   `json:"free_text,omitempty"`
}

type RuntimeRequest struct {
	ID         string
	Info       *AskInfo
	Metadata   runtimeport.InterruptMetadata
	ToolCallID string
	ToolName   string
}

type RuntimeHandler func(context.Context, RuntimeRequest) (*AskResponse, error)

func WithRuntimeHandler(ctx context.Context, handler RuntimeHandler) context.Context {
	if handler == nil {
		return ctx
	}
	return context.WithValue(ctx, handlerContextKey{}, handler)
}

func runtimeHandlerFromContext(ctx context.Context) (RuntimeHandler, bool) {
	handler, ok := ctx.Value(handlerContextKey{}).(RuntimeHandler)
	return handler, ok
}

// AskRequest 工具输入参数
type AskRequest struct {
	Question    string   `json:"question" jsonschema:"description=要向用户提出的问题"`
	Options     []string `json:"options,omitempty" jsonschema:"description=可供用户选择的选项列表（可选）"`
	MultiSelect bool     `json:"multi_select,omitempty" jsonschema:"description=是否允许多选（默认单选）"`
}

// AskResult 工具返回结果
type AskResult struct {
	Selected []string `json:"selected,omitempty"`
	FreeText string   `json:"free_text,omitempty"`
}

// AskQuestions 执行提问
func AskQuestions(ctx context.Context, req *AskRequest) (*AskResult, error) {
	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}

	info := &AskInfo{
		Question:    req.Question,
		Options:     req.Options,
		MultiSelect: req.MultiSelect,
	}

	if handler, ok := runtimeHandlerFromContext(ctx); ok {
		if metadata, hasMetadata := runtimeport.InterruptMetadataFromContext(ctx); hasMetadata && metadata.MemberCallID != "" {
			toolMetadata, _ := runtimeport.ToolRuntimeMetadataFromContext(ctx)
			resp, err := handler(ctx, RuntimeRequest{
				ID:         uuid.NewString(),
				Info:       info,
				Metadata:   metadata,
				ToolCallID: toolMetadata.CallID,
				ToolName:   toolMetadata.Name,
			})
			if err != nil {
				return nil, err
			}
			if resp == nil {
				return nil, fmt.Errorf("no response received")
			}
			return &AskResult{
				Selected: resp.Selected,
				FreeText: resp.FreeText,
			}, nil
		}
	}

	// 检查是否从中断恢复
	wasInterrupted, _, _ := runtimeport.GetInterruptState(ctx)
	if wasInterrupted {
		isTarget, hasData, resp := runtimeport.GetResumeContext[*AskResponse](ctx)
		if !isTarget {
			return nil, runtimeport.RequestInterrupt(ctx, nil)
		}
		if hasData && resp != nil {
			return &AskResult{
				Selected: resp.Selected,
				FreeText: resp.FreeText,
			}, nil
		}
		return nil, fmt.Errorf("no response received")
	}

	// 首次调用，触发中断
	return nil, runtimeport.RequestInterrupt(ctx, info)
}

// GetTools 返回 askQuestions 工具
func GetTools() ([]runtimeport.Tool, error) {
	askTool, err := runtimeport.InferTool(
		"ask_questions",
		`向用户提出问题，收集用户输入、观点或让用户做出选择。

仅在你确实被用户决策阻塞时使用：这个答案必须会改变下一步行动，且无法从请求、代码、历史上下文或合理默认值中判断。

不要用于：
- 询问显而易见的默认选择
- 让用户确认你是否应该继续
- 代替你自己阅读代码、配置或工具结果
- 一次提出大量泛泛问题

使用要求：
- 问题要具体，直接说明需要用户决定什么
- 有选项时提供 2-4 个互斥选项
- 推荐选项放在第一位，并在文本中标注“推荐”
- 用户也可以自由输入文本回答`,
		AskQuestions,
	)
	if err != nil {
		return nil, err
	}
	return []runtimeport.Tool{askTool}, nil
}

// Package ask 提供 askQuestions 工具，允许模型向用户提问以收集信息、观点或做出选择。
package ask

import (
	"context"
	"encoding/gob"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

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
	Selected []string `json:"selected,omitempty"`
	FreeText string   `json:"free_text,omitempty"`
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

	// 检查是否从中断恢复
	wasInterrupted, _, _ := tool.GetInterruptState[any](ctx)
	if wasInterrupted {
		isTarget, hasData, resp := tool.GetResumeContext[*AskResponse](ctx)
		if !isTarget {
			return nil, tool.Interrupt(ctx, nil)
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
	return nil, tool.Interrupt(ctx, info)
}

// GetTools 返回 askQuestions 工具
func GetTools() ([]tool.BaseTool, error) {
	askTool, err := utils.InferTool(
		"ask_questions",
		"向用户提出问题，收集用户输入、观点或让用户做出选择。可以提供选项列表供用户选择（单选或多选），用户也可以自由输入文本回答。当你需要用户确认方案、补充信息或表达偏好时使用此工具。",
		AskQuestions,
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{askTool}, nil
}

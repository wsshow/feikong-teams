package dispatch

import (
	"context"
	"encoding/json"
	rootcommon "fkteams/common"
	"fkteams/fkevent"
	"fkteams/tools/approval"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const (
	statusSuccess = "success"
	statusError   = "error"
	statusTimeout = "timeout"
)

type dispatchInput struct {
	Tasks []taskItem `json:"tasks" jsonschema:"description=子任务列表"`
}

type taskItem struct {
	Description string `json:"description" jsonschema:"description=子任务的详细描述，要足够清晰以便独立执行"`
}

type taskResult struct {
	TaskIndex   int      `json:"task_index"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Result      string   `json:"result,omitempty"`
	Error       string   `json:"error,omitempty"`
	Operations  []string `json:"operations,omitempty"`
}

func (m *middleware) executeTasks(ctx context.Context, input *dispatchInput) (string, error) {
	if len(input.Tasks) == 0 {
		return `{"results":[]}`, nil
	}

	// 通知任务清单
	var list strings.Builder
	for i, t := range input.Tasks {
		fmt.Fprintf(&list, "\n  [%d] %s", i, t.Description)
	}
	emit(ctx, "", "action",
		fmt.Sprintf("正在并行分发 %d 个子任务（自主执行模式，操作将自动审批）:%s", len(input.Tasks), list.String()))

	results := make([]taskResult, len(input.Tasks))
	sem := semaphore.NewWeighted(m.maxConcurrency)
	g, gCtx := errgroup.WithContext(ctx)
	var mu sync.Mutex

	for i, task := range input.Tasks {
		g.Go(func() error {
			if err := sem.Acquire(gCtx, 1); err != nil {
				mu.Lock()
				results[i] = taskResult{TaskIndex: i, Description: task.Description, Status: statusError, Error: "cancelled"}
				mu.Unlock()
				return nil
			}
			defer sem.Release(1)

			r := m.executeOneTask(ctx, i, task)
			mu.Lock()
			results[i] = r
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	data, err := json.Marshal(struct {
		Results []taskResult `json:"results"`
	}{results})
	if err != nil {
		return "", fmt.Errorf("marshal results: %w", err)
	}
	return string(data), nil
}

func (m *middleware) executeOneTask(parentCtx context.Context, index int, task taskItem) taskResult {
	agentName := fmt.Sprintf("子任务-%d", index)
	result := taskResult{TaskIndex: index, Description: task.Description}

	emit(parentCtx, agentName, "action", fmt.Sprintf("开始执行: %s", task.Description))

	taskCtx, cancel := context.WithTimeout(parentCtx, m.taskTimeout)
	defer cancel()

	taskCtx = approval.WithRegistry(taskCtx, approval.NewAutoApproveRegistry())

	agent, err := m.createSubAgent(taskCtx, agentName, task.Description)
	if err != nil {
		return fail(result, statusError, fmt.Sprintf("create agent: %v", err))
	}

	output, ops, err := m.runAgent(parentCtx, taskCtx, agent, agentName, task.Description)
	if err != nil {
		if taskCtx.Err() != nil {
			emit(parentCtx, agentName, "action", fmt.Sprintf("超时: %s", task.Description))
			return fail(result, statusTimeout, "task timeout")
		}
		emit(parentCtx, agentName, "error", err.Error())
		return fail(result, statusError, err.Error())
	}

	emit(parentCtx, agentName, "action", fmt.Sprintf("完成: %s", task.Description))
	result.Status = statusSuccess
	result.Result = output
	result.Operations = ops
	return result
}

func (m *middleware) createSubAgent(ctx context.Context, name, desc string) (adk.Agent, error) {
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        name,
		Description: fmt.Sprintf("执行子任务: %s", desc),
		Instruction: subAgentInstruction,
		Model:       m.chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: m.tools},
		},
		MaxIterations:    defaultMaxIterations,
		ModelRetryConfig: &adk.ModelRetryConfig{MaxRetries: defaultMaxRetries},
	})
}

// runAgent 执行子智能体，转发事件并收集输出和操作记录
func (m *middleware) runAgent(parentCtx, taskCtx context.Context, agent adk.Agent, agentName, desc string) (string, []string, error) {
	runner := adk.NewRunner(taskCtx, adk.RunnerConfig{
		Agent: agent, EnableStreaming: false,
		CheckPointStore: rootcommon.NewInMemoryStore(),
	})

	iter := runner.Run(taskCtx, []*schema.Message{{Role: schema.User, Content: desc}})

	var content strings.Builder
	var operations []string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return "", operations, event.Err
		}

		// 转发子智能体事件
		event.AgentName = agentName
		_ = fkevent.ProcessAgentEvent(parentCtx, event)

		// 收集操作记录
		operations = append(operations, extractOperations(event)...)

		if msg := extractMessage(event); msg != nil && msg.Content != "" {
			content.WriteString(msg.Content)
		}
	}
	return content.String(), operations, nil
}

func fail(r taskResult, status, errMsg string) taskResult {
	r.Status = status
	r.Error = errMsg
	return r
}

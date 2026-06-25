package schedule

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appchat "fkteams/internal/app/chat"
	"fkteams/internal/domain/message"
	runtimeport "fkteams/internal/ports/runtime"
)

// RunnerCreator 为每次后台任务创建独立运行器。
type RunnerCreator func(ctx context.Context) (runtimeport.Runner, error)

// BackgroundExecutor 将调度任务转换为一次后台聊天运行。
type BackgroundExecutor struct {
	createRunner RunnerCreator
	resultsDir   string
	chat         *appchat.Service
}

// NewBackgroundExecutor 创建后台任务执行器。
func NewBackgroundExecutor(createRunner RunnerCreator, resultsDir string) *BackgroundExecutor {
	_ = os.MkdirAll(resultsDir, 0755)
	return &BackgroundExecutor{
		createRunner: createRunner,
		resultsDir:   resultsDir,
		chat:         appchat.NewService(),
	}
}

func (e *BackgroundExecutor) taskDir(taskID string) string {
	return filepath.Join(e.resultsDir, taskID)
}

func (e *BackgroundExecutor) taskResultPath(taskID string) string {
	return filepath.Join(e.taskDir(taskID), "result.md")
}

// Execute 执行调度任务并写入当前结果和历史快照。
func (e *BackgroundExecutor) Execute(ctx context.Context, taskID string, task string) (string, error) {
	if err := os.MkdirAll(e.taskDir(taskID), 0755); err != nil {
		return "", fmt.Errorf("create task dir: %w", err)
	}
	if e.createRunner == nil {
		return "", fmt.Errorf("create runner: runner creator is nil")
	}

	r, err := e.createRunner(ctx)
	if err != nil {
		return "", fmt.Errorf("create runner: %w", err)
	}

	callback, getResult := newMarkdownCollector()
	input := message.TurnInput{
		Message: message.Message{Role: message.RoleUser, Content: task},
	}

	_, err = e.chat.RunTurn(ctx, appchat.TurnRequest{
		SessionID:    "fkteams_scheduler",
		Runner:       r,
		Input:        input,
		EventHandler: callback,
	})
	if err != nil {
		errMsg := fmt.Sprintf("execution error: %v", err)
		e.writeResult(taskID, task, errMsg)
		return "", err
	}

	output := getResult()
	e.writeResult(taskID, task, output)
	return output, nil
}

func (e *BackgroundExecutor) writeResult(taskID string, task string, result string) {
	now := time.Now()
	ts := now.Format("20060102_150405")

	content := fmt.Sprintf("# Task Result\n\n**Task ID**: %s\n\n**Time**: %s\n\n**Task**: %s\n\n## Result\n\n%s\n",
		taskID,
		now.Format("2006-01-02 15:04:05"),
		task,
		result,
	)

	_ = os.WriteFile(e.taskResultPath(taskID), []byte(content), 0644)

	historyDir := filepath.Join(e.taskDir(taskID), "history")
	_ = os.MkdirAll(historyDir, 0755)
	_ = os.WriteFile(filepath.Join(historyDir, ts+".md"), []byte(content), 0644)
}

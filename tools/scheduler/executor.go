package scheduler

import (
	"context"
	"fkteams/engine"
	"fkteams/fkevent"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// RunnerCreator creates a Runner for task execution
type RunnerCreator func(ctx context.Context) (*adk.Runner, error)

// BackgroundExecutor executes tasks in the background
type BackgroundExecutor struct {
	createRunner RunnerCreator
	resultsDir   string
}

// NewBackgroundExecutor creates a background executor
func NewBackgroundExecutor(createRunner RunnerCreator, resultsDir string) *BackgroundExecutor {
	_ = os.MkdirAll(resultsDir, 0755)
	return &BackgroundExecutor{
		createRunner: createRunner,
		resultsDir:   resultsDir,
	}
}

// taskDir returns the per-task result directory
func (e *BackgroundExecutor) taskDir(taskID string) string {
	return filepath.Join(e.resultsDir, taskID)
}

// taskResultPath returns the path to a task's result file
func (e *BackgroundExecutor) taskResultPath(taskID string) string {
	return filepath.Join(e.taskDir(taskID), "result.md")
}

// Execute runs a task and writes the result to the per-task directory
func (e *BackgroundExecutor) Execute(ctx context.Context, taskID string, task string) (string, error) {
	if err := os.MkdirAll(e.taskDir(taskID), 0755); err != nil {
		return "", fmt.Errorf("create task dir: %w", err)
	}

	r, err := e.createRunner(ctx)
	if err != nil {
		return "", fmt.Errorf("create runner: %w", err)
	}

	callback, getResult := fkevent.NewMarkdownCollector()

	inputMessages := []adk.Message{schema.UserMessage(task)}

	_, err = engine.New(r, "fkteams_scheduler").Run(ctx, engine.RunConfig{
		Messages:      inputMessages,
		EventCallback: callback,
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

// writeResult writes the task result to the per-task directory
func (e *BackgroundExecutor) writeResult(taskID string, task string, result string) {
	content := fmt.Sprintf("# Task Result\n\n**Task ID**: %s\n\n**Time**: %s\n\n**Task**: %s\n\n## Result\n\n%s\n",
		taskID,
		time.Now().Format("2006-01-02 15:04:05"),
		task,
		result,
	)

	_ = os.WriteFile(e.taskResultPath(taskID), []byte(content), 0644)
}

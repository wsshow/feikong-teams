package schedule

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"

	appchat "fkteams/internal/app/chat"
	"fkteams/internal/domain/message"
	domainschedule "fkteams/internal/domain/schedule"
	domainsession "fkteams/internal/domain/session"
	runtimeport "fkteams/internal/ports/runtime"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"
)

// RunnerCreator 为每次后台任务创建独立运行器。
type RunnerCreator func(ctx context.Context) (runtimeport.Runner, error)

// BackgroundExecutor 将调度任务转换为一次后台聊天运行。
type BackgroundExecutor struct {
	createRunner RunnerCreator
	resultsDir   string
	chat         *appchat.Service
	contextHook  func(context.Context) context.Context
}

// NewBackgroundExecutor 创建后台任务执行器。
func NewBackgroundExecutor(createRunner RunnerCreator, resultsDir string) (*BackgroundExecutor, error) {
	absResultsDir, err := filepath.Abs(resultsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve scheduler results directory: %w", err)
	}
	if err := os.MkdirAll(absResultsDir, 0755); err != nil {
		return nil, fmt.Errorf("create scheduler results directory: %w", err)
	}
	root, err := os.OpenRoot(absResultsDir)
	if err != nil {
		return nil, fmt.Errorf("open scheduler results directory: %w", err)
	}
	if err := root.Close(); err != nil {
		return nil, fmt.Errorf("close scheduler results directory: %w", err)
	}
	return &BackgroundExecutor{
		createRunner: createRunner,
		resultsDir:   absResultsDir,
		chat:         appchat.NewService(),
	}, nil
}

// WithContextHook 设置每次执行前的上下文装配逻辑。
func (e *BackgroundExecutor) WithContextHook(hook func(context.Context) context.Context) *BackgroundExecutor {
	e.contextHook = hook
	return e
}

func (e *BackgroundExecutor) taskDir(taskID string) string {
	return filepath.Join(e.resultsDir, taskID)
}

func (e *BackgroundExecutor) taskResultPath(taskID string) string {
	return filepath.Join(e.taskDir(taskID), "result.md")
}

// Execute 执行调度任务并写入当前结果和历史快照。
func (e *BackgroundExecutor) Execute(ctx context.Context, taskID string, task string) (string, error) {
	if !domainsession.ValidID(taskID) || len(taskID) > 160 {
		return "", fmt.Errorf("invalid task ID")
	}
	if e.contextHook != nil {
		ctx = e.contextHook(ctx)
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
		SessionID: "fkteams_scheduler_" + taskID,
		Runner:    r,
		Input:     input,
		EventSink: callback,
	})
	if err != nil {
		errMsg := fmt.Sprintf("execution error: %v", err)
		if writeErr := e.writeResult(taskID, task, errMsg); writeErr != nil {
			return "", errors.Join(err, writeErr)
		}
		return "", err
	}

	output := getResult()
	if err := e.writeResult(taskID, task, output); err != nil {
		return "", err
	}
	return output, nil
}

func (e *BackgroundExecutor) writeResult(taskID string, task string, result string) error {
	now := time.Now()
	ts := now.Format("20060102_150405")

	header := fmt.Sprintf("# Task Result\n\n**Task ID**: %s\n\n**Time**: %s\n\n**Task**: %s\n\n## Result\n\n",
		taskID,
		now.Format("2006-01-02 15:04:05"),
		task,
	)
	available := domainschedule.MaxResultFileBytes - len(header) - 1
	if available < len(truncatedOutputMarker) {
		return fmt.Errorf("task result metadata exceeds size limit")
	}
	result = truncateResult(result, available)
	content := header + result + "\n"

	root, err := os.OpenRoot(e.resultsDir)
	if err != nil {
		return fmt.Errorf("open scheduler results directory: %w", err)
	}
	defer root.Close()
	historyPath := filepath.Join(taskID, "history")
	if err := pathguard.EnsureRootDirectory(root, historyPath, 0755); err != nil {
		return fmt.Errorf("create task result directories: %w", err)
	}
	if err := atomicfile.WriteFileInRoot(root, filepath.Join(taskID, "result.md"), []byte(content), 0644); err != nil {
		return fmt.Errorf("write task result: %w", err)
	}

	if err := atomicfile.WriteFileInRoot(root, filepath.Join(historyPath, ts+".md"), []byte(content), 0644); err != nil {
		return fmt.Errorf("write task history: %w", err)
	}
	if err := pruneTaskHistory(root, historyPath); err != nil {
		return fmt.Errorf("prune task history: %w", err)
	}
	return nil
}

func truncateResult(result string, limit int) string {
	if len(result) <= limit {
		return result
	}
	cut := limit - len(truncatedOutputMarker)
	for cut > 0 && !utf8.ValidString(result[:cut]) {
		cut--
	}
	return result[:cut] + truncatedOutputMarker
}

func pruneTaskHistory(root *os.Root, historyPath string) error {
	directory, err := root.Open(historyPath)
	if err != nil {
		return err
	}
	names := make([]string, 0, domainschedule.MaxHistoryEntries+1)
	scanned := 0
	for {
		entries, readErr := directory.ReadDir(256)
		scanned += len(entries)
		if scanned > domainschedule.MaxHistoryScanEntries {
			directory.Close()
			return fmt.Errorf("history directory exceeds scan limit")
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".md" {
				names = append(names, entry.Name())
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			directory.Close()
			return readErr
		}
	}
	if err := directory.Close(); err != nil {
		return err
	}
	if len(names) <= domainschedule.MaxHistoryEntries {
		return nil
	}
	sort.Strings(names)
	for _, name := range names[:len(names)-domainschedule.MaxHistoryEntries] {
		if err := root.Remove(filepath.Join(historyPath, name)); err != nil {
			return err
		}
	}
	return nil
}

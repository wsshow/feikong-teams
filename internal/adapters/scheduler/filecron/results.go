package filecron

import (
	"errors"
	domainschedule "fkteams/internal/domain/schedule"
	domainsession "fkteams/internal/domain/session"
	"fkteams/internal/runtime/atomicfile"
	"fkteams/internal/runtime/pathguard"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"
)

// ResultStore 管理调度任务结果文件。
type ResultStore struct{ resultsDir string }

// NewResultStore 校验并创建结果存储目录。
func NewResultStore(resultsDir string) (*ResultStore, error) {
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
	return &ResultStore{resultsDir: absResultsDir}, nil
}
func (e *ResultStore) taskDir(taskID string) string {
	return filepath.Join(e.resultsDir, taskID)
}

func (e *ResultStore) taskResultPath(taskID string) string {
	return filepath.Join(e.taskDir(taskID), "result.md")
}

// SaveResult 原子写入当前结果并保留有界历史。
func (e *ResultStore) SaveResult(taskID string, task string, result string) error {
	if !domainsession.ValidID(taskID) || len(taskID) > 160 {
		return fmt.Errorf("invalid task ID")
	}
	now := time.Now()
	ts := now.Format("20060102_150405.000000000")

	header := fmt.Sprintf("# Task Result\n\n**Task ID**: %s\n\n**Time**: %s\n\n**Task**: %s\n\n## Result\n\n",
		taskID,
		now.Format("2006-01-02 15:04:05"),
		task,
	)
	available := domainschedule.MaxResultFileBytes - len(header) - 1
	if available < len("\n\n> Output truncated because it exceeded the scheduler result limit.") {
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
	cut := limit - len("\n\n> Output truncated because it exceeded the scheduler result limit.")
	for cut > 0 && !utf8.ValidString(result[:cut]) {
		cut--
	}
	return result[:cut] + "\n\n> Output truncated because it exceeded the scheduler result limit."
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

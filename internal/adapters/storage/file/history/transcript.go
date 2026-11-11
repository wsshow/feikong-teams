package eventlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"fkteams/internal/runtime/atomicfile"

	"github.com/google/uuid"
)

const (
	toolResultsDirName = "tool-results"
	subagentsDirName   = "subagents"

	longToolResultChars = 4000
	resultSummaryChars  = 1200

	maxTranscriptRecordBytes = 16 << 20
	maxSubagentMetadataBytes = 1 << 20
)

func newPrefixedID(prefix string) string {
	return prefix + "_" + uuid.NewString()
}

func transcriptIDPrefix(typ TranscriptEventType) string {
	switch typ {
	case TranscriptUserMessage, TranscriptAssistantMessage:
		return "msg"
	case TranscriptAgentStep:
		return "step"
	case TranscriptToolCallStart, TranscriptToolCallEnd:
		return "tool"
	case TranscriptAskRequested, TranscriptAskAnswered:
		return "ask"
	case TranscriptError:
		return "err"
	case TranscriptSystemNotice:
		return "notice"
	case TranscriptCancelled:
		return "cancel"
	default:
		return "rec"
	}
}

func transcriptPath(sessionDir string) string {
	return filepath.Join(sessionDir, TranscriptFileName)
}

func subagentTranscriptPath(sessionDir, agentRunID string) string {
	return filepath.Join(sessionDir, subagentsDirName, filepath.Base(agentRunID), TranscriptFileName)
}

func subagentMetadataPath(sessionDir, agentRunID string) string {
	return filepath.Join(sessionDir, subagentsDirName, filepath.Base(agentRunID), "metadata.json")
}

func writeSubagentMetadata(sessionDir string, metadata SubagentMetadata) error {
	if sessionDir == "" || metadata.AgentRunID == "" {
		return nil
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode subagent metadata: %w", err)
	}
	if err := atomicfile.WriteFile(subagentMetadataPath(sessionDir, metadata.AgentRunID), data, 0644); err != nil {
		return fmt.Errorf("write subagent metadata: %w", err)
	}
	return nil
}

func loadSubagentMetadata(filePath string) (SubagentMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return SubagentMetadata{}, fmt.Errorf("read subagent metadata: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSubagentMetadataBytes+1))
	if err != nil {
		return SubagentMetadata{}, fmt.Errorf("read subagent metadata: %w", err)
	}
	if len(data) > maxSubagentMetadataBytes {
		return SubagentMetadata{}, fmt.Errorf("read subagent metadata: file exceeds %d bytes", maxSubagentMetadataBytes)
	}
	var metadata SubagentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return SubagentMetadata{}, fmt.Errorf("decode subagent metadata: %w", err)
	}
	return metadata, nil
}

func (h *HistoryRecorder) appendTranscriptEvent(event TranscriptEvent, subagent *subagentRun) {
	if h == nil || h.sessionDir == "" || h.persistErr != nil {
		return
	}
	if event.ID == "" {
		event.ID = newPrefixedID(transcriptIDPrefix(event.Type))
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	if subagent == nil {
		if err := appendJSONL(transcriptPath(h.sessionDir), event); err != nil {
			h.persistErr = err
		}
		return
	}
	if event.Agent == "" {
		event.Agent = subagent.AgentName
	}
	if err := appendJSONL(subagent.TranscriptPath, event); err != nil {
		h.persistErr = err
	}
}

func appendJSONL(filePath string, value any) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open jsonl: %w", err)
	}
	if err := json.NewEncoder(file).Encode(value); err != nil {
		_ = file.Close()
		return fmt.Errorf("append jsonl: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync jsonl: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close jsonl: %w", err)
	}
	return nil
}

func loadTranscript(filePath string) ([]TranscriptEvent, error) {
	return loadTranscriptRecords(filePath, false)
}

func loadTranscriptForResume(filePath string) ([]TranscriptEvent, error) {
	return loadTranscriptRecords(filePath, true)
}

func loadTranscriptRecords(filePath string, repairTail bool) ([]TranscriptEvent, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var events []TranscriptEvent
	var offset int64
	var lastGoodOffset int64
	truncateTo := int64(-1)
	appendNewline := false
	for line := 1; ; line++ {
		record, readErr := readTranscriptRecord(reader, maxTranscriptRecordBytes)
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("read transcript record %d: %w", line, readErr)
		}
		offset += int64(len(record))
		trimmed := bytes.TrimSpace(record)
		if len(trimmed) > 0 {
			var event TranscriptEvent
			if err := json.Unmarshal(trimmed, &event); err != nil {
				// 进程异常退出可能留下未完成的最后一行，保留此前完整记录即可继续恢复。
				if readErr == io.EOF {
					if repairTail {
						truncateTo = lastGoodOffset
					}
					break
				}
				return nil, fmt.Errorf("decode transcript record %d: %w", line, err)
			}
			events = append(events, event)
		}
		lastGoodOffset = offset
		if readErr == io.EOF {
			appendNewline = repairTail && len(record) > 0 && record[len(record)-1] != '\n'
			break
		}
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close transcript: %w", err)
	}
	if truncateTo >= 0 {
		if err := os.Truncate(filePath, truncateTo); err != nil {
			return nil, fmt.Errorf("repair transcript tail: %w", err)
		}
	} else if appendNewline {
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open transcript tail: %w", err)
		}
		if _, err := file.WriteString("\n"); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("repair transcript newline: %w", err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close transcript tail: %w", err)
		}
	}
	return events, nil
}

func readTranscriptRecord(reader *bufio.Reader, limit int) ([]byte, error) {
	record := make([]byte, 0, min(reader.Size(), limit))
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > limit-len(record) {
			return nil, fmt.Errorf("record exceeds %d bytes", limit)
		}
		record = append(record, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return record, err
	}
}

func shouldExternalizeToolResult(toolName, content string) bool {
	if utf8.RuneCountInString(content) > longToolResultChars {
		return true
	}
	for _, prefix := range NoisyToolPrefixes {
		if strings.HasPrefix(toolName, prefix) && utf8.RuneCountInString(content) > resultSummaryChars {
			return true
		}
	}
	return false
}

func summarizeToolResult(content string) string {
	content = strings.TrimSpace(content)
	runes := []rune(content)
	if len(runes) <= resultSummaryChars {
		return content
	}
	return string(runes[:resultSummaryChars]) + "\n...(truncated)..."
}

type toolResultPayload struct {
	Result        string
	ResultRef     string
	Summary       string
	Truncated     bool
	OriginalChars int
}

func (h *HistoryRecorder) toolResultPayload(toolName, content string) toolResultPayload {
	payload := toolResultPayload{
		Result: content,
	}
	if h == nil || h.sessionDir == "" || content == "" || !shouldExternalizeToolResult(toolName, content) {
		return payload
	}
	id := newPrefixedID("result")
	rel := filepath.Join(toolResultsDirName, id+".json")
	artifact := ToolResultArtifact{
		ID:            id,
		ToolName:      toolName,
		Content:       content,
		Summary:       summarizeToolResult(content),
		OriginalChars: utf8.RuneCountInString(content),
		CreatedAt:     time.Now(),
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return payload
	}
	if err := atomicfile.WriteFile(filepath.Join(h.sessionDir, rel), data, 0644); err != nil {
		return payload
	}
	payload.Result = ""
	payload.ResultRef = rel
	payload.Summary = artifact.Summary
	payload.Truncated = true
	payload.OriginalChars = artifact.OriginalChars
	return payload
}

func usageRecordFromEvent(event Event) *UsageRecord {
	promptTokens := event.PromptTokens
	completionTokens := event.CompletionTokens
	totalTokens := event.TotalTokens
	var cachedTokens, cacheWriteTokens int
	if event.Usage != nil {
		cachedTokens = event.Usage.CachedTokens
		cacheWriteTokens = event.Usage.CacheWriteTokens
		if promptTokens == 0 {
			promptTokens = event.Usage.PromptTokens
		}
		if completionTokens == 0 {
			completionTokens = event.Usage.CompletionTokens
		}
		if totalTokens == 0 {
			totalTokens = event.Usage.TotalTokens
		}
	}
	if promptTokens == 0 && completionTokens == 0 && totalTokens == 0 && cachedTokens == 0 && cacheWriteTokens == 0 {
		return nil
	}
	return &UsageRecord{
		PromptTokens: promptTokens, CompletionTokens: completionTokens, TotalTokens: totalTokens,
		CachedTokens: cachedTokens, CacheWriteTokens: cacheWriteTokens,
	}
}

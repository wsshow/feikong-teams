package eventlog

import (
	"encoding/json"

	"fkteams/common/atomicfile"

	"fmt"
	"io"
	"os"
	"path/filepath"

	"strings"

	"time"
)

func (h *HistoryRecorder) SaveToFile(filePath string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return saveMessagesToFile(h.snapshotMessagesLocked(), filePath)
}

func (h *HistoryRecorder) snapshotMessagesLocked() []AgentMessage {
	messages := make([]AgentMessage, len(h.messages))
	copy(messages, h.messages)
	for _, key := range h.activeOrder {
		ctx := h.activeMessages[key]
		if ctx == nil || len(ctx.msg.Events) == 0 {
			continue
		}
		msg := ctx.msg
		msg.EndTime = time.Now()
		messages = append(messages, msg)
	}
	return messages
}

func saveMessagesToFile(messages []AgentMessage, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	data, err := marshalMessagesJSONL(messages)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func marshalMessagesJSONL(messages []AgentMessage) ([]byte, error) {
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	for msgIndex, msg := range messages {
		messageID := historyMessageID(msg, msgIndex)
		for eventIndex, event := range msg.Events {
			line := HistoryLine{
				Type:           historyLineTypeMessageEvent,
				MessageID:      messageID,
				EventIndex:     eventIndex,
				AgentName:      msg.AgentName,
				RunPath:        msg.RunPath,
				MemberCallID:   msg.MemberCallID,
				MemberToolName: msg.MemberToolName,
				MemberName:     msg.MemberName,
				StartTime:      msg.StartTime,
				EndTime:        msg.EndTime,
				Event:          event,
			}
			if err := encoder.Encode(line); err != nil {
				return nil, fmt.Errorf("marshal jsonl: %w", err)
			}
		}
	}
	return []byte(builder.String()), nil
}

func historyMessageID(msg AgentMessage, index int) string {
	return fmt.Sprintf("%06d:%s:%s", index, msg.AgentName, msg.StartTime.UTC().Format(time.RFC3339Nano))
}

func (h *HistoryRecorder) LoadFromFile(filePath string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	defer file.Close()

	messages, err := loadMessagesJSONL(file)
	if err != nil {
		return err
	}
	h.messages = messages

	h.reconstructSummaryFromEvents()

	h.activeMessages = make(map[string]*activeMessageContext)
	h.activeOrder = nil

	return nil
}

func loadMessagesJSONL(file *os.File) ([]AgentMessage, error) {
	messages := make([]AgentMessage, 0)
	messageIndex := make(map[string]int)
	decoder := json.NewDecoder(file)
	lineNo := 1
	for {
		var line HistoryLine
		if err := decoder.Decode(&line); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode jsonl record %d: %w", lineNo, err)
		}
		if line.Type != historyLineTypeMessageEvent {
			return nil, fmt.Errorf("unsupported history line type at record %d: %s", lineNo, line.Type)
		}
		if line.MessageID == "" {
			return nil, fmt.Errorf("missing message_id at record %d", lineNo)
		}
		idx, exists := messageIndex[line.MessageID]
		if !exists {
			messageIndex[line.MessageID] = len(messages)
			messages = append(messages, AgentMessage{
				AgentName:      line.AgentName,
				RunPath:        line.RunPath,
				MemberCallID:   line.MemberCallID,
				MemberToolName: line.MemberToolName,
				MemberName:     line.MemberName,
				StartTime:      line.StartTime,
				EndTime:        line.EndTime,
				Events:         make([]MessageEvent, 0),
			})
			idx = len(messages) - 1
		}
		messages[idx].Events = append(messages[idx].Events, line.Event)
		messages[idx].EndTime = line.EndTime
		lineNo++
	}
	return messages, nil
}

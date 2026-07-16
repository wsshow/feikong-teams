package inputhistory

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"fkteams/internal/runtime/atomicfile"
)

const (
	maxStoredHistoryLines = 100
	maxHistoryLineBytes   = 64 << 10
	maxHistoryFileBytes   = 16 << 20
	historyLinePrefix     = "@fkteams:v1:"
)

// Save 将最近的输入历史原子保存到文件。
func Save(filePath string, history []string) error {
	if len(history) > maxStoredHistoryLines {
		history = history[len(history)-maxStoredHistoryLines:]
	}
	var builder strings.Builder
	for _, line := range history {
		line = truncateHistoryLine(line)
		builder.WriteString(historyLinePrefix)
		builder.WriteString(base64.RawStdEncoding.EncodeToString([]byte(line)))
		builder.WriteByte('\n')
	}
	if err := atomicfile.WriteFile(filePath, []byte(builder.String()), 0644); err != nil {
		return fmt.Errorf("save input history: %w", err)
	}
	return nil
}

// Load 从文件加载输入历史，最多返回 maxLines 行。
func Load(filePath string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		return []string{}, nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxHistoryFileBytes {
		return nil, fmt.Errorf("input history exceeds %d bytes", maxHistoryFileBytes)
	}

	maxEncodedLineBytes := base64.RawStdEncoding.EncodedLen(maxHistoryLineBytes) + len(historyLinePrefix)
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxEncodedLineBytes+1)
	for scanner.Scan() {
		line := decodeHistoryLine(scanner.Text())
		lines = append(lines, line)
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func decodeHistoryLine(line string) string {
	encoded, ok := strings.CutPrefix(line, historyLinePrefix)
	if !ok {
		return truncateHistoryLine(line)
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return truncateHistoryLine(line)
	}
	return truncateHistoryLine(string(decoded))
}

func truncateHistoryLine(line string) string {
	line = strings.ToValidUTF8(line, "�")
	if len(line) <= maxHistoryLineBytes {
		return line
	}
	line = line[:maxHistoryLineBytes]
	for line != "" && !utf8.ValidString(line) {
		line = line[:len(line)-1]
	}
	return line
}

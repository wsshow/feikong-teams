// Package scriptexec 提供脚本工具共用的子进程执行边界。
package scriptexec

import (
	"bytes"
	"fmt"
	"os/exec"
	"sync"
	"unicode/utf8"
)

const (
	MaxOutputBytes  int64 = 1 << 20
	truncatedMarker       = "\n[output truncated]"
)

// CombinedOutput 执行命令并限制 stdout 与 stderr 的合计内存占用。
func CombinedOutput(command *exec.Cmd, limit int64) ([]byte, bool, error) {
	if command == nil {
		return nil, false, fmt.Errorf("command is nil")
	}
	if limit < 0 {
		return nil, false, fmt.Errorf("output limit must not be negative")
	}
	if command.Stdout != nil || command.Stderr != nil {
		return nil, false, fmt.Errorf("command output is already configured")
	}

	output := &limitedBuffer{remaining: limit}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	data, truncated := output.result()
	return data, truncated, err
}

// String 将有限输出转换为适合工具响应的文本，并显式标记截断。
func String(output []byte, truncated bool) string {
	if !truncated {
		return string(output)
	}
	for len(output) > 0 && !utf8.Valid(output) {
		output = output[:len(output)-1]
	}
	return string(output) + truncatedMarker
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int64
	truncated bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	originalLength := len(data)
	if int64(len(data)) > buffer.remaining {
		data = data[:buffer.remaining]
		buffer.truncated = true
	}
	if len(data) > 0 {
		_, _ = buffer.buffer.Write(data)
		buffer.remaining -= int64(len(data))
	}
	return originalLength, nil
}

func (buffer *limitedBuffer) result() ([]byte, bool) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return bytes.Clone(buffer.buffer.Bytes()), buffer.truncated
}

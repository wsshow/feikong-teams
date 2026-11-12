// Package executil 提供子进程执行的共享资源边界。
package executil

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"
)

const truncatedMarker = "\n[output truncated]"

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

	ConfigureProcessGroup(command)
	output := &limitedBuffer{remaining: limit}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	data, truncated := output.result()
	return data, truncated, err
}

// String 将有限输出转换为文本，并显式标记截断。
func String(output []byte, truncated bool) string {
	if !truncated {
		return string(output)
	}
	// 只去除尾部未完成的字符；内部非法字节由一次线性转换处理。
	for len(output) > 0 && !utf8.FullRune(output[lastRuneStart(output):]) {
		output = output[:lastRuneStart(output)]
	}

	return strings.ToValidUTF8(string(output), "�") + truncatedMarker
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

func lastRuneStart(data []byte) int {
	i := len(data) - 1
	for i > 0 && !utf8.RuneStart(data[i]) {
		i--
	}
	return max(i, 0)
}

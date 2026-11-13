// Package local 提供进程组执行和受根目录约束的任务记录、循环日志。
package local

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	domain "fkteams/internal/domain/process"
	port "fkteams/internal/ports/process"
	"fkteams/internal/runtime/atomicfile"
)

const logCapacity = 4 << 20
const tailCapacity = 8 << 10

// Backend 同时实现进程、日志和记录端口，目录由应用实例独占。
type Backend struct{ root *os.Root }

// New 打开任务目录；调用方必须在持有应用存储锁后创建。
func New(directory string) (*Backend, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Backend{root: root}, nil
}

// Close 在任务排空之后关闭目录句柄。
func (b *Backend) Close() error { return b.root.Close() }

func validID(id string) bool {
	if !strings.HasPrefix(id, "proc_") || len(id) != 41 {
		return false
	}
	for _, c := range id[5:] {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func (b *Backend) open(id, suffix string, flags int) (*os.File, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid process ID")
	}
	return b.root.OpenFile(id+suffix, flags, 0600)
}

// Save 原子持久化任务记录。
func (b *Backend) Save(record domain.Record) error {
	if !validID(record.ID) {
		return fmt.Errorf("invalid process ID")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return atomicfile.WriteFileInRoot(b.root, record.ID+".json", data, 0600)
}

// Load 加载记录并清除尚未成功提交启动记录的孤立日志。
func (b *Backend) Load() ([]domain.Record, error) {
	dir, err := b.root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, err := dir.ReadDir(-1)
	closeErr := dir.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	var records []domain.Record
	known := make(map[string]bool)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		file, err := b.open(id, ".json", os.O_RDONLY)
		if err != nil {
			return nil, err
		}
		var record domain.Record
		err = json.NewDecoder(io.LimitReader(file, 256<<10)).Decode(&record)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		if record.ID != id {
			return nil, fmt.Errorf("process record ID mismatch")
		}
		records = append(records, record)
		known[id] = true
	}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".log")
		if strings.HasSuffix(entry.Name(), ".log") && validID(id) && !known[id] {
			if err := b.Remove(id); err != nil {
				return nil, err
			}
		}
	}
	return records, nil
}

// Delete 移除已经过保留期的任务记录。
func (b *Backend) Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("invalid process ID")
	}
	err := b.root.Remove(id + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Remove 只删除指定任务拥有的日志。
func (b *Backend) Remove(id string) error {
	if !validID(id) {
		return fmt.Errorf("invalid process ID")
	}
	err := b.root.Remove(id + ".log")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type logFile struct {
	mu             sync.Mutex
	file           *os.File
	total          int64
	stdout, stderr []byte
	err            error
}

type streamWriter struct {
	log    *logFile
	stderr bool
}

func (w streamWriter) Write(data []byte) (int, error) {
	l := w.log
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return 0, l.err
	}
	n := len(data)
	tail := &l.stdout
	if w.stderr {
		tail = &l.stderr
	}
	*tail = append(*tail, data[max(0, n-tailCapacity):]...)
	if len(*tail) > tailCapacity {
		*tail = append([]byte(nil), (*tail)[len(*tail)-tailCapacity:]...)
	}
	start := l.total
	l.total += int64(n)
	if len(data) > logCapacity {
		data = data[len(data)-logCapacity:]
		start = l.total - logCapacity
	}
	position := start % logCapacity
	first := min(len(data), logCapacity-int(position))
	_, err := l.file.WriteAt(data[:first], 8+position)
	if err == nil && first < len(data) {
		_, err = l.file.WriteAt(data[first:], 8)
	}
	if err == nil {
		var header [8]byte
		binary.LittleEndian.PutUint64(header[:], uint64(l.total))
		_, err = l.file.WriteAt(header[:], 0)
	}
	l.err = err
	return n, err
}

func readLog(file *os.File, total, cursor int64, limit int) (port.Output, error) {
	if limit < 1 || limit > 64<<10 || cursor < -1 {
		return port.Output{}, fmt.Errorf("invalid output cursor or limit")
	}
	from := max(int64(0), total-logCapacity)
	wanted := cursor
	if cursor == -1 {
		cursor = max(from, total-int64(limit))
		wanted = cursor
	}
	cursor = min(max(cursor, from), total)
	data := make([]byte, min(int64(limit), total-cursor))
	position := cursor % logCapacity
	first := min(len(data), logCapacity-int(position))
	if first > 0 {
		if _, err := file.ReadAt(data[:first], 8+position); err != nil {
			return port.Output{}, err
		}
	}
	if first < len(data) {
		if _, err := file.ReadAt(data[first:], 8); err != nil {
			return port.Output{}, err
		}
	}
	// 常规分页不切断 UTF-8 字符；极小预算或非法原始字节仍以替换字符展示。
	if cursor+int64(len(data)) < total {
		for i := len(data) - 1; i >= max(0, len(data)-utf8.UTFMax); i-- {
			if utf8.RuneStart(data[i]) {
				if i > 0 && !utf8.FullRune(data[i:]) {
					data = data[:i]
				}
				break
			}
		}
	}
	return port.Output{Text: strings.ToValidUTF8(string(data), "�"), From: cursor, Next: cursor + int64(len(data)), Total: total, Truncated: wanted < from}, nil
}

// Read 通过字节游标读取任务的合并日志；-1 读取最新尾部。
func (b *Backend) Read(id string, _ int64, cursor int64, limit int) (port.Output, error) {
	file, err := b.open(id, ".log", os.O_RDONLY)
	if err != nil {
		return port.Output{}, err
	}
	defer file.Close()
	var header [8]byte
	if _, err := file.ReadAt(header[:], 0); err != nil {
		return port.Output{}, err
	}
	total := int64(binary.LittleEndian.Uint64(header[:]))
	if total < 0 {
		return port.Output{}, fmt.Errorf("invalid log header")
	}
	return readLog(file, total, cursor, limit)
}

type process struct {
	cmd        *exec.Cmd
	log        *logFile
	done       chan struct{}
	cancelDone chan struct{}
	backend    *Backend
	id         string
}

// Start 在独立进程组中执行命令，日志自启动起落盘。
func (b *Backend) Start(ctx context.Context, spec port.Spec) (port.Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := b.open(spec.ID, ".log", os.O_CREATE|os.O_EXCL|os.O_RDWR)
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(make([]byte, 8)); err != nil {
		file.Close()
		b.Remove(spec.ID)
		return nil, err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NonInteractive", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $OutputEncoding = [System.Text.Encoding]::UTF8; "+spec.Command)
	} else {
		cmd = exec.Command("/bin/sh", "-l", "-c", spec.Command)
		if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
			cmd = exec.Command("/bin/bash", "-l", "-c", spec.Command)
		}
	}
	cmd.Dir = spec.Directory
	configure(cmd)
	l := &logFile{file: file}
	cmd.Stdout, cmd.Stderr = streamWriter{log: l}, streamWriter{log: l, stderr: true}
	if err := cmd.Start(); err != nil {
		file.Close()
		b.Remove(spec.ID)
		return nil, err
	}
	p := &process{cmd: cmd, log: l, done: make(chan struct{}), cancelDone: make(chan struct{}), backend: b, id: spec.ID}
	go func() {
		defer close(p.cancelDone)
		select {
		case <-ctx.Done():
			_ = terminate(cmd)
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-p.done:
			}
			// shell 已退出也可能留下忽略 TERM 的后代，仍清理本进程组。
			_ = kill(cmd)
		case <-p.done:
			_ = kill(cmd)
		}
	}()
	return p, nil
}

func (p *process) PID() int { return p.cmd.Process.Pid }
func (p *process) Read(cursor int64, limit int) (port.Output, error) {
	p.log.mu.Lock()
	defer p.log.mu.Unlock()
	return p.backend.Read(p.id, p.log.total, cursor, limit)
}
func (p *process) Wait() port.Result {
	err := p.cmd.Wait()
	close(p.done)
	<-p.cancelDone
	p.log.mu.Lock()
	defer p.log.mu.Unlock()
	logErr := errors.Join(p.log.err, p.log.file.Sync())
	result := port.Result{ExitCode: p.cmd.ProcessState.ExitCode(), OutputBytes: p.log.total, Stdout: strings.ToValidUTF8(string(p.log.stdout), "�"), Stderr: strings.ToValidUTF8(string(p.log.stderr), "�")}
	// Read 仍可在任务保留期内通过 backend 重新打开日志。
	logErr = errors.Join(logErr, p.log.file.Close())
	if err != nil && result.ExitCode == 0 {
		result.ExitCode = -1
	}
	if err != nil {
		result.Error = err.Error()
	}
	if logErr != nil {
		result.Error = errors.Join(err, logErr).Error()
		result.ExitCode = -1
	}
	return result
}

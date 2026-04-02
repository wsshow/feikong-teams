package mdiff

import "testing"

// memAccessor 是测试用的内存文件系统
type memAccessor struct {
	files map[string]string
}

func (m *memAccessor) ReadFile(path string) (string, error) {
	content, ok := m.files[path]
	if !ok {
		return "", &ApplyError{File: path, Message: "file not found"}
	}
	return content, nil
}

func (m *memAccessor) WriteFile(path string, content string) error {
	m.files[path] = content
	return nil
}

func (m *memAccessor) DeleteFile(path string) error {
	if _, ok := m.files[path]; !ok {
		return &ApplyError{File: path, Message: "file not found"}
	}
	delete(m.files, path)
	return nil
}

// roundTripTest 验证 diff → format → parse → apply 往返一致性
func roundTripTest(t *testing.T, oldContent, newContent string) {
	t.Helper()

	fd := DiffFiles("test.txt", oldContent, "test.txt", newContent, 3)
	patchStr := FormatFileDiff(fd)

	parsed, err := ParseFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	result, err := ApplyFileDiff(oldContent, parsed)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result != newContent {
		t.Errorf("round-trip mismatch:\nold:    %q\nwant:   %q\ngot:    %q\npatch:\n%s",
			oldContent, newContent, result, patchStr)
	}
}

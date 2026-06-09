package log

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestSprintf(t *testing.T) {
	if got := Sprintf("hello %s", "world"); got != "hello world" {
		t.Fatalf("Sprintf() = %q, want hello world", got)
	}
}

func TestReadLevelDefaultsToDebug(t *testing.T) {
	if got := readLevel(); got != zapcore.DebugLevel {
		t.Fatalf("readLevel() = %v, want debug", got)
	}
}

func TestLoggerWritesToAppLogFile(t *testing.T) {
	resetLogger(t)
	appDir := t.TempDir()
	t.Setenv("FEIKONG_APP_DIR", appDir)

	Info("hello log")
	Printf("formatted %s", "message")
	Println("line log")
	Print("plain log")
	Debug("debug log")
	Warn("warn log")
	Error("error log")
	Debugf("debug %s", "format")
	Infof("info %s", "format")
	Warnf("warn %s", "format")
	Errorf("error %s", "format")
	if sugar != nil {
		_ = sugar.Sync()
	}

	data, err := os.ReadFile(filepath.Join(appDir, "log", "fkteams.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)
	for _, want := range []string{"hello log", "formatted message", "line log", "plain log", "debug log", "warn log", "error log"} {
		if !strings.Contains(content, want) {
			t.Fatalf("log content = %q, missing %q", content, want)
		}
	}
}

func TestLoggerIsSingleton(t *testing.T) {
	resetLogger(t)
	t.Setenv("FEIKONG_APP_DIR", t.TempDir())

	first := logger()
	second := logger()
	if first == nil || second == nil {
		t.Fatal("logger returned nil")
	}
	if first != second {
		t.Fatal("logger should return singleton instance")
	}
}

func resetLogger(t *testing.T) {
	t.Helper()

	sugar = nil
	once = sync.Once{}
	t.Cleanup(func() {
		if sugar != nil {
			_ = sugar.Sync()
		}
		sugar = nil
		once = sync.Once{}
	})
}

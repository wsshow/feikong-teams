package g

import (
	"testing"

	"fkteams/common"
)

func TestProcessCleanerInitialized(t *testing.T) {
	if ProcessCleaner == nil {
		t.Fatal("ProcessCleaner should be initialized")
	}
}

func TestRunProcessCleanupExecutesAndClearsCallbacks(t *testing.T) {
	original := ProcessCleaner
	ProcessCleaner = common.NewResourceCleaner()
	t.Cleanup(func() {
		ProcessCleaner = original
	})

	var calls []string
	ProcessCleaner.Add(func() error {
		calls = append(calls, "first")
		return nil
	})
	ProcessCleaner.Add(func() error {
		calls = append(calls, "second")
		return nil
	})

	RunProcessCleanup()
	if len(calls) != 2 || calls[0] != "second" || calls[1] != "first" {
		t.Fatalf("cleanup calls = %v, want LIFO order", calls)
	}

	RunProcessCleanup()
	if len(calls) != 2 {
		t.Fatalf("cleanup ran again after clear: %v", calls)
	}
}

package eventlog

import (
	"fkteams/internal/domain/event"
	"fmt"
	"sync"
	"testing"
)

func TestRecordAndEmitOrdersConcurrentMemberInteractions(t *testing.T) {
	recorder := NewHistoryRecorder()
	recorder.SetSessionDir(t.TempDir())
	var observed []int64
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for index := 0; index < 16; index++ {
				e := Event{Type: event.TypeSystemNotice, Content: fmt.Sprintf("worker-%d-%d", worker, index)}
				if err := recorder.RecordAndEmit(e, func(recorded Event) error { observed = append(observed, recorded.SessionSequence); return nil }); err != nil {
					t.Error(err)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	if len(observed) != 64 {
		t.Fatalf("events=%d", len(observed))
	}
	for i, sequence := range observed {
		if sequence != int64(i+1) {
			t.Fatalf("published sequence %d at index %d", sequence, i)
		}
	}
}

package weixin

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	channel "fkteams/internal/adapters/transport/channel"
)

func TestTypingIndicatorsAreBoundedAndCancelled(t *testing.T) {
	c := &Channel{typingCancels: make(map[string]typingIndicator)}
	var cancelled atomic.Int32
	for i := 0; i <= maxTypingIndicators; i++ {
		c.registerTyping(fmt.Sprintf("user-%d", i), func() { cancelled.Add(1) })
	}
	c.typingMu.Lock()
	count := len(c.typingCancels)
	c.typingMu.Unlock()
	if count != maxTypingIndicators {
		t.Fatalf("typing indicator count = %d, want %d", count, maxTypingIndicators)
	}
	if cancelled.Load() != 1 {
		t.Fatalf("evicted indicator cancel count = %d, want 1", cancelled.Load())
	}

	c.cancelAllTyping()
	if cancelled.Load() != maxTypingIndicators+1 {
		t.Fatalf("total cancel count = %d, want %d", cancelled.Load(), maxTypingIndicators+1)
	}
}

func TestTypingReplacementKeepsNewestRegistration(t *testing.T) {
	c := &Channel{typingCancels: make(map[string]typingIndicator)}
	var firstCancelled atomic.Bool
	var secondCancelled atomic.Bool
	firstID := c.registerTyping("user", func() { firstCancelled.Store(true) })
	secondID := c.registerTyping("user", func() { secondCancelled.Store(true) })
	if !firstCancelled.Load() {
		t.Fatal("previous typing indicator was not cancelled")
	}
	c.removeTyping("user", firstID)
	c.typingMu.Lock()
	indicator, exists := c.typingCancels["user"]
	c.typingMu.Unlock()
	if !exists || indicator.id != secondID {
		t.Fatal("old typing goroutine removed the newest registration")
	}
	c.stopTyping("user")
	if !secondCancelled.Load() {
		t.Fatal("newest typing indicator was not cancelled")
	}
}

func TestStopCancelsLifecycleAndTyping(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	close(done)
	var typingCancelled atomic.Bool
	c := &Channel{
		cancel:  cancel,
		runCtx:  runCtx,
		runDone: done,
		typingCancels: map[string]typingIndicator{
			"user": {cancel: func() { typingCancelled.Store(true) }},
		},
	}
	c.running.Store(true)
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("stop channel: %v", err)
	}
	if runCtx.Err() == nil || !typingCancelled.Load() {
		t.Fatal("stop did not cancel lifecycle and typing contexts")
	}
	if c.IsRunning() {
		t.Fatal("channel should not remain running after stop")
	}
}

func TestSendRejectsStoppedChannel(t *testing.T) {
	c := &Channel{typingCancels: make(map[string]typingIndicator)}
	err := c.Send(context.Background(), "user", channel.Message{Content: "hello"})
	if err == nil {
		t.Fatal("expected stopped channel send to fail")
	}
}

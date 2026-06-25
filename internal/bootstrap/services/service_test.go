package services

import (
	"fkteams/internal/app/appstate"
	"testing"
)

func TestServiceConstructors(t *testing.T) {
	state := appstate.New()
	memoryService := NewMemoryService("/tmp/work", state)
	if memoryService.Name() != "memory" || memoryService.workspaceDir != "/tmp/work" || memoryService.state != state {
		t.Fatalf("memory service = %#v", memoryService)
	}

	schedulerService := NewSchedulerService("/tmp/scheduler")
	if schedulerService.Name() != "scheduler" || schedulerService.schedulerDir != "/tmp/scheduler" {
		t.Fatalf("scheduler service = %#v", schedulerService)
	}
}

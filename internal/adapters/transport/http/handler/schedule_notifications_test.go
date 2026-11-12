package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appschedule "fkteams/internal/app/schedule"
	"fkteams/internal/domain/apperror"
	schedulerport "fkteams/internal/ports/scheduler"
	"github.com/gin-gonic/gin"
)

type notificationScheduler struct {
	schedulerport.TaskService
	runAt        time.Time
	acknowledged bool
}

func (s *notificationScheduler) AcknowledgeResult(_ context.Context, _ string, runAt time.Time) error {
	if !runAt.Equal(s.runAt) {
		return apperror.New(apperror.CodeConflict, "newer result exists")
	}
	s.acknowledged = true
	return nil
}

func TestNotificationAcknowledgmentHTTP(t *testing.T) {
	backend := &notificationScheduler{runAt: time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)}
	rt := &Runtime{RuntimeOptions: RuntimeOptions{Scheduler: appschedule.NewService(backend)}}
	router := gin.New()
	router.POST("/schedules/:id/acknowledge", rt.AcknowledgeScheduleResultHandler())
	cases := []struct {
		body   string
		status int
	}{
		{`{}`, http.StatusBadRequest},
		{`{"run_at":"invalid"}`, http.StatusBadRequest},
		{`{"run_at":"2026-11-11T10:00:00Z"}`, http.StatusConflict},
		{`{"run_at":"2026-11-12T10:00:00Z"}`, http.StatusOK},
		{`{"run_at":"2026-11-12T10:00:00Z"}`, http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/schedules/report/acknowledge", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != tc.status {
			t.Fatalf("body %s: got %d %s", tc.body, resp.Code, resp.Body.String())
		}
		if tc.status != http.StatusOK && backend.acknowledged {
			t.Fatal("invalid acknowledgment was applied")
		}
	}
	if !backend.acknowledged {
		t.Fatal("notification not acknowledged")
	}
}

package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"fkteams/internal/domain/apperror"
	domainsession "fkteams/internal/domain/session"
)

type memoryRepository struct {
	items map[string]domainsession.Metadata
}

func (r *memoryRepository) ListSessions(context.Context) ([]domainsession.Record, error) {
	result := make([]domainsession.Record, 0, len(r.items))
	for _, metadata := range r.items {
		result = append(result, domainsession.Record{Metadata: metadata})
	}
	return result, nil
}

func (r *memoryRepository) CreateSession(_ context.Context, metadata domainsession.Metadata) (domainsession.Metadata, bool, error) {
	if existing, ok := r.items[metadata.ID]; ok {
		return existing, false, nil
	}
	r.items[metadata.ID] = metadata
	return metadata, true, nil
}

func (r *memoryRepository) LoadSession(_ context.Context, id string) (domainsession.Metadata, error) {
	metadata, ok := r.items[id]
	if !ok {
		return domainsession.Metadata{}, apperror.New(apperror.CodeNotFound, "session not found")
	}
	return metadata, nil
}

func (r *memoryRepository) UpdateSession(_ context.Context, id string, update func(*domainsession.Metadata) error) (domainsession.Metadata, error) {
	metadata, ok := r.items[id]
	if !ok {
		return domainsession.Metadata{}, apperror.New(apperror.CodeNotFound, "session not found")
	}
	if err := update(&metadata); err != nil {
		return domainsession.Metadata{}, err
	}
	r.items[id] = metadata
	return metadata, nil
}

func (r *memoryRepository) DeleteSession(_ context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return apperror.New(apperror.CodeNotFound, "session not found")
	}
	delete(r.items, id)
	return nil
}

func TestCreateAndPatchUseConsistentNormalization(t *testing.T) {
	repository := &memoryRepository{items: make(map[string]domainsession.Metadata)}
	service := NewService(repository)
	fixedNow := time.Unix(100, 0)
	service.now = func() time.Time { return fixedNow }

	longTitle := strings.Repeat("题", 55)
	metadata, created, err := service.Create(context.Background(), CreateRequest{SessionID: "session-1", Title: longTitle})
	if err != nil || !created {
		t.Fatalf("create failed: created=%v err=%v", created, err)
	}
	if len([]rune(metadata.Title)) != 53 {
		t.Fatalf("normalized title length = %d", len([]rune(metadata.Title)))
	}

	favorite := true
	agent := " coder "
	updated, err := service.Update(context.Background(), UpdateRequest{SessionID: "session-1", Favorite: &favorite, CurrentAgent: &agent})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Favorite || updated.CurrentAgent != "coder" || !updated.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("unexpected metadata: %#v", updated)
	}
}

func TestUpdateRequiresAtLeastOneField(t *testing.T) {
	repository := &memoryRepository{items: map[string]domainsession.Metadata{
		"session-1": {ID: "session-1"},
	}}
	_, err := NewService(repository).Update(context.Background(), UpdateRequest{SessionID: "session-1"})
	if !apperror.IsCode(err, apperror.CodeInvalidArgument) {
		t.Fatalf("unexpected error: %v", err)
	}
}

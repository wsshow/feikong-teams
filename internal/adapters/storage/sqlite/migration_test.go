package sqlite

import (
	"context"
	domainmigration "fkteams/internal/domain/migration"
	"path/filepath"
	"testing"
)

func TestMigrationIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	progress := func(string, int, int) {}
	document := domainmigration.Document{Path: "sessions/one/metadata.json", Data: []byte(`{"id":"one"}`)}
	if err := db.Import(ctx, domainmigration.Snapshot{Documents: []domainmigration.Document{document, document}}, progress); err == nil {
		t.Fatal("duplicate import accepted")
	}
	complete, err := db.Completed(ctx)
	if err != nil || complete {
		t.Fatal("failed migration marked complete")
	}
	if err := db.Import(ctx, domainmigration.Snapshot{Documents: []domainmigration.Document{document}}, progress); err != nil {
		t.Fatal(err)
	}
	if err := db.Import(ctx, domainmigration.Snapshot{}, progress); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.db.QueryRow("SELECT count(*) FROM records").Scan(&count); err != nil || count != 1 {
		t.Fatal("retry changed committed records")
	}
}

func TestMigrationCancellationRollsBackAllWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	document := domainmigration.Document{Path: "sessions/one/metadata.json", Data: []byte(`{"id":"one"}`)}
	if err := db.Import(ctx, domainmigration.Snapshot{Documents: []domainmigration.Document{document}}, func(stage string, _, _ int) {
		if stage == "import" {
			cancel()
		}
	}); err == nil {
		t.Fatal("cancelled import succeeded")
	}
	if complete, err := db.Completed(context.Background()); err != nil || complete {
		t.Fatal("cancelled import committed")
	}
	if err := db.Import(context.Background(), domainmigration.Snapshot{Documents: []domainmigration.Document{document}}, func(string, int, int) {}); err != nil {
		t.Fatal("partial writes survived rollback", err)
	}
}

package migrate_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aidapedia/hayat/internal/migrate"
	"github.com/aidapedia/hayat/internal/testdb"
)

func TestRunIsIdempotent(t *testing.T) {
	s := testdb.New(t) // already migrated once
	ctx := context.Background()
	if err := migrate.Run(ctx, s, testdb.Engine(), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("second run: %v", err)
	}
	status, err := migrate.Status(ctx, s, testdb.Engine())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range status {
		if !strings.HasPrefix(line, "applied") {
			t.Errorf("expected every migration applied, got %q", line)
		}
	}
}

func TestEditedMigrationIsRefused(t *testing.T) {
	s := testdb.New(t)
	ctx := context.Background()
	if _, err := s.Pool.Exec(ctx, `UPDATE hayat_migrations SET checksum = 'tampered' WHERE version = '0001_init'`); err != nil {
		t.Fatal(err)
	}
	err := migrate.Run(ctx, s, testdb.Engine(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("want an append-only error, got %v", err)
	}
}

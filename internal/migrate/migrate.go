// Package migrate applies the embedded migration history for the configured
// engine.
//
// Migrations are append-only. Each applied file's SHA-256 is recorded, and a
// recorded file whose content has since changed stops the run: editing a
// shipped migration would leave databases that ran the old text and databases
// that ran the new one silently different.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/migrations"
)

type file struct {
	version  string
	sql      string
	checksum string
}

func load(engine config.Engine) ([]file, error) {
	dir := string(engine)
	entries, err := fs.ReadDir(migrations.FS, dir)
	if err != nil {
		return nil, fmt.Errorf("migrate: no history for engine %q: %w", engine, err)
	}
	var files []file
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		files = append(files, file{
			version:  strings.TrimSuffix(e.Name(), ".sql"),
			sql:      string(body),
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files, nil
}

// Run applies every pending migration, each in its own transaction.
func Run(ctx context.Context, s *store.Store, engine config.Engine, log *slog.Logger) error {
	files, err := load(engine)
	if err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS hayat_migrations (
		version    text PRIMARY KEY,
		checksum   text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	applied := map[string]string{}
	rows, err := s.Pool.Query(ctx, `SELECT version, checksum FROM hayat_migrations`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for rows.Next() {
		var v, c string
		if err := rows.Scan(&v, &c); err != nil {
			rows.Close()
			return err
		}
		applied[v] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, f := range files {
		if sum, ok := applied[f.version]; ok {
			if sum != f.checksum {
				return fmt.Errorf("migrate: %s/%s.sql changed after it was applied; migrations are append-only, add a new file instead", engine, f.version)
			}
			continue
		}
		err := s.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, f.sql); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO hayat_migrations (version, checksum) VALUES ($1, $2)`, f.version, f.checksum)
			return err
		})
		if err != nil {
			return fmt.Errorf("migrate: %s: %w", f.version, err)
		}
		log.Info("applied migration", "engine", engine, "version", f.version)
	}
	return nil
}

// Status lists each migration and whether it has been applied.
func Status(ctx context.Context, s *store.Store, engine config.Engine) ([]string, error) {
	files, err := load(engine)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	rows, err := s.Pool.Query(ctx, `SELECT version FROM hayat_migrations`)
	if err == nil {
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				applied[v] = true
			}
		}
		rows.Close()
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		state := "pending"
		if applied[f.version] {
			state = "applied"
		}
		out = append(out, fmt.Sprintf("%-8s %s", state, f.version))
	}
	return out, nil
}

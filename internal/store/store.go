// Package store is Hayat's access to its database.
//
// The same SQL runs on PostgreSQL and YugabyteDB. Two rules here keep it that
// way: every transaction goes through Tx, which retries serialization failures
// (a distributed database returns them far more often), and ids are UUIDv7 so
// no table funnels its writes through one sequence.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the subset of pgx both a pool and a transaction offer, so a module's
// queries run the same inside or outside a transaction.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Store struct {
	Pool   *pgxpool.Pool
	Region string
}

func Open(ctx context.Context, url, region string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: cannot reach the database: %w", err)
	}
	return &Store{Pool: pool, Region: region}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// ErrNotFound is returned when a row a caller asked for by key doesn't exist.
var ErrNotFound = errors.New("not found")

const maxAttempts = 5

// Tx runs fn in a transaction and retries it when the database reports a
// serialization failure or deadlock. fn must therefore be safe to run more than
// once: no side effects outside the transaction.
func (s *Store) Tx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = pgx.BeginFunc(ctx, s.Pool, fn)
		if !retryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt*attempt) * 10 * time.Millisecond):
		}
	}
	return err
}

func retryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	// 40001 serialization_failure, 40P01 deadlock_detected.
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

// IsUniqueViolation reports whether err is a unique-constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// NewID returns a UUIDv7: time-ordered, so inserts stay local in an index,
// without a sequence every writer contends on.
func NewID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

// NotFound maps pgx's no-rows error to ErrNotFound and passes others through.
func NotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

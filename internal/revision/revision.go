// Package revision counts policy changes, per tenant.
//
// Every change to a tenant's tree, grants or rosters bumps that tenant's
// revision and records a policy event, in the transaction that made the change.
// Caches put the revision in their keys, so a change in one tenant never cools
// another's cache. Changes that belong to no tenant (the catalog, roles, grants
// on the root or a service node, an account being disabled) bump the global
// revision, which every cache key also carries.
package revision

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/store"
)

// Global is the tenant id of the global revision.
var Global = uuid.Nil

// Bump increments a revision and records the event. tenantID may be Global.
func Bump(ctx context.Context, db store.DB, region string, tenantID uuid.UUID, kind string) (int64, error) {
	var rev int64
	err := db.QueryRow(ctx, `INSERT INTO tenant_revisions (region, tenant_id, rev) VALUES ($1, $2, 1)
		ON CONFLICT (region, tenant_id) DO UPDATE SET rev = tenant_revisions.rev + 1
		RETURNING rev`, region, tenantID).Scan(&rev)
	if err != nil {
		return 0, err
	}
	_, err = db.Exec(ctx, `INSERT INTO policy_events (region, id, tenant_id, rev, kind) VALUES ($1, $2, $3, $4, $5)`,
		region, store.NewID(), tenantID, rev, kind)
	return rev, err
}

// Get reads a revision without caching. A tenant with no row is at 0.
func Get(ctx context.Context, db store.DB, tenantID uuid.UUID) (int64, error) {
	var rev int64
	err := db.QueryRow(ctx, `SELECT coalesce(max(rev), 0) FROM tenant_revisions WHERE tenant_id = $1`, tenantID).Scan(&rev)
	return rev, err
}

// Reader caches revisions for a short time, so a burst of checks costs one
// read per tenant rather than one per check. Watch events and at_least_rev
// cover the window.
type Reader struct {
	store *store.Store
	ttl   time.Duration

	mu    sync.Mutex
	cache map[uuid.UUID]cached
}

type cached struct {
	rev int64
	at  time.Time
}

func NewReader(s *store.Store, ttl time.Duration) *Reader {
	return &Reader{store: s, ttl: ttl, cache: map[uuid.UUID]cached{}}
}

// Get returns the tenant's revision, at least atLeast: a cached value older than
// that is re-read. fresh skips the cache entirely.
func (r *Reader) Get(ctx context.Context, tenantID uuid.UUID, atLeast int64, fresh bool) (int64, error) {
	if !fresh {
		r.mu.Lock()
		c, ok := r.cache[tenantID]
		r.mu.Unlock()
		if ok && time.Since(c.at) < r.ttl && c.rev >= atLeast {
			return c.rev, nil
		}
	}
	rev, err := Get(ctx, r.store.Pool, tenantID)
	if err != nil {
		return 0, err
	}
	r.mu.Lock()
	r.cache[tenantID] = cached{rev: rev, at: time.Now()}
	r.mu.Unlock()
	return rev, nil
}

// Forget drops a cached revision, after this process changed it.
func (r *Reader) Forget(tenantID uuid.UUID) {
	r.mu.Lock()
	delete(r.cache, tenantID)
	r.mu.Unlock()
}

type Event struct {
	ID       uuid.UUID
	TenantID uuid.UUID
	Rev      int64
	Kind     string
	At       time.Time
}

// Since lists events after the cursor (a UUIDv7, so ordered by time), oldest
// first. An empty cursor starts from now.
func Since(ctx context.Context, db store.DB, after uuid.UUID, limit int) ([]Event, error) {
	rows, err := db.Query(ctx, `SELECT id, tenant_id, rev, kind, created_at FROM policy_events
		WHERE id > $1 ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Rev, &e.Kind, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Cursor returns an id that sorts just before events made from now on.
func Cursor() uuid.UUID {
	id := store.NewID()
	// Zero the random bits: every v7 id minted after this instant sorts after it.
	for i := 8; i < 16; i++ {
		id[i] = 0
	}
	id[6] &= 0xf0
	id[7] = 0
	return id
}

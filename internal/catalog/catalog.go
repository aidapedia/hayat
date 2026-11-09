// Package catalog is what Hayat knows about the world: node types, the
// permissions that mean something on them, and the roles that bundle
// permissions. Hayat's own entries are code (platform.go); every service adds
// its own by publishing a contract (contract.go).
package catalog

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

type NodeType struct {
	Key                string
	Service            string
	Parents            []string
	AccessPermission   string
	ViaPlacement       bool
	RequiresMembership bool
	Degradable         bool
	Description        string
}

type Permission struct {
	Key         string
	Service     string
	Scope       string
	Description string
}

type Role struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	Service     string
	AppliesTo   []string
	Permissions []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GrantableOn reports whether the role may be granted on a node of this type.
func (r Role) GrantableOn(nodeType string) bool {
	return len(r.AppliesTo) == 0 || slices.Contains(r.AppliesTo, nodeType)
}

// Snapshot is the catalog at one global revision. It is read-only.
type Snapshot struct {
	Rev         int64
	Types       map[string]NodeType
	Permissions map[string]Permission
	Roles       map[uuid.UUID]Role
}

// Known reports whether a permission key exists. A grant holding an unknown
// key is ignored, never honoured.
func (s *Snapshot) Known(perm string) bool { _, ok := s.Permissions[perm]; return ok }

// CanParent reports whether a node of type parent may hold one of type child.
func (s *Snapshot) CanParent(parent, child string) bool {
	t, ok := s.Types[child]
	return ok && slices.Contains(t.Parents, parent)
}

// CanContain reports whether a node of type ancestor can, at any depth, hold a
// node of type descendant. It decides where a permission can be granted: at its
// scope type or anywhere above it.
func (s *Snapshot) CanContain(ancestor, descendant string) bool {
	if ancestor == descendant {
		return true
	}
	seen := map[string]bool{}
	queue := []string{descendant}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, p := range s.Types[cur].Parents {
			if p == ancestor {
				return true
			}
			if !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return false
}

// RolesWith returns the ids of roles that carry a permission.
func (s *Snapshot) RolesWith(perm string) []uuid.UUID {
	var out []uuid.UUID
	for id, r := range s.Roles {
		if slices.Contains(r.Permissions, perm) {
			out = append(out, id)
		}
	}
	return out
}

// TypesForPermission lists the node types whose access permission is perm.
func (s *Snapshot) AccessTypes(perm string) []NodeType {
	var out []NodeType
	for _, t := range s.Types {
		if t.AccessPermission == perm {
			out = append(out, t)
		}
	}
	return out
}

// ServiceOf returns the namespace of a type or permission key.
func ServiceOf(key string) string {
	if i := strings.IndexAny(key, "/."); i > 0 {
		return key[:i]
	}
	return key
}

// Service reads the catalog and caches it by global revision.
type Service struct {
	store *store.Store
	revs  *revision.Reader

	mu   sync.Mutex
	snap *Snapshot
}

func New(s *store.Store, revs *revision.Reader) *Service { return &Service{store: s, revs: revs} }

// Bootstrap writes the platform catalog. Every pod calls it at start-up.
func (c *Service) Bootstrap(ctx context.Context) error {
	return c.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM tenant_revisions WHERE tenant_id = $1 FOR UPDATE`, revision.Global); err != nil {
			return err
		}
		return EnsurePlatform(ctx, tx)
	})
}

// Snapshot returns the current catalog, reloading it when the global revision
// moved. fresh forces a re-read of the revision.
func (c *Service) Snapshot(ctx context.Context, fresh bool) (*Snapshot, error) {
	rev, err := c.revs.Get(ctx, revision.Global, 0, fresh)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	cur := c.snap
	c.mu.Unlock()
	if cur != nil && cur.Rev == rev {
		return cur, nil
	}
	snap, err := load(ctx, c.store.Pool, rev)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.snap = snap
	c.mu.Unlock()
	return snap, nil
}

func load(ctx context.Context, db store.DB, rev int64) (*Snapshot, error) {
	s := &Snapshot{Rev: rev, Types: map[string]NodeType{}, Permissions: map[string]Permission{}, Roles: map[uuid.UUID]Role{}}
	rows, err := db.Query(ctx, `SELECT key, service_id, parents, access_permission, via_placement, requires_membership, degradable, description FROM node_types`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t NodeType
		if err := rows.Scan(&t.Key, &t.Service, &t.Parents, &t.AccessPermission, &t.ViaPlacement, &t.RequiresMembership, &t.Degradable, &t.Description); err != nil {
			rows.Close()
			return nil, err
		}
		s.Types[t.Key] = t
	}
	rows.Close()

	rows, err = db.Query(ctx, `SELECT key, service_id, scope, description FROM permissions`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Key, &p.Service, &p.Scope, &p.Description); err != nil {
			rows.Close()
			return nil, err
		}
		s.Permissions[p.Key] = p
	}
	rows.Close()

	roles, err := listRoles(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		// Keys the catalog no longer knows are dropped, never honoured.
		r.Permissions = slices.DeleteFunc(r.Permissions, func(p string) bool { return !s.Known(p) })
		s.Roles[r.ID] = r
	}
	return s, nil
}

func listRoles(ctx context.Context, db store.DB) ([]Role, error) {
	rows, err := db.Query(ctx, `SELECT r.id, r.slug, r.name, r.description, r.service_id, r.applies_to, r.created_at, r.updated_at,
		coalesce(array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL), '{}')
		FROM roles r LEFT JOIN role_permissions rp ON rp.role_id = r.id
		GROUP BY r.id ORDER BY r.service_id = 'hayat' DESC, r.service_id, r.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.Description, &r.Service, &r.AppliesTo, &r.CreatedAt, &r.UpdatedAt, &r.Permissions); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

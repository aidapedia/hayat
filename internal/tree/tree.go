// Package tree owns the resource tree's structure: nodes, group rosters and
// grants. It decides what shapes are allowed; whether a *caller* may make a
// change is decided by the API layer through the authz engine.
//
// Rules:
//   - A node may only sit under a type its catalog entry lists as a parent.
//   - A node is deleted only when nothing is under it. No silent re-parenting.
//   - A node mirroring a service's resource is created, renamed, moved and
//     deleted by that service only.
//   - Groups nest like an org chart: whoever is on a group's roster counts in
//     every group above it (membership.go). Moving a group therefore changes
//     which grants reach its people; the admin API asks for the right to
//     grant where it lands.
//   - A grant to a user needs them on the tenant's roster; a grant to a group
//     names a group of the same tenant, or the tenant itself ("everyone").
//   - A role is granted only where it applies, and each of its permissions only
//     at its scope or above.
//   - Every change bumps the tenant's revision (the global one outside tenants).
package tree

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

const MaxDepth = 16

var (
	ErrNotEmpty      = errs.New("something is still filed under this node; move or delete it first")
	ErrWrongParent   = errs.New("this kind of node can't be placed there")
	ErrCycle         = errs.New("a node can't be moved under itself")
	ErrTooDeep       = errs.New("the tree would be deeper than 16 levels")
	ErrNotGroup      = errs.New("only groups can be changed this way")
	ErrNotMember     = errs.New("that person isn't a member of this tenant")
	ErrNotYourType   = errs.New("a service can only manage its own resource types")
	ErrGrantExists   = errs.New("that role is already granted to them here")
	ErrNameMissing   = errs.New("give it a name")
	ErrOutsideTenant = errs.New("resources live inside a tenant")
)

type Node struct {
	ID         uuid.UUID
	Region     string
	TenantID   *uuid.UUID
	ParentID   *uuid.UUID
	Type       string
	ExternalID string
	Name       string
	OwnerID    *uuid.UUID
	Path       string
	Depth      int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Chain is the ids from the root down to and including the node.
func (n Node) Chain() []uuid.UUID {
	out := []uuid.UUID{catalog.RootID}
	for _, seg := range strings.Split(strings.Trim(n.Path, "/"), "/") {
		if id, err := uuid.Parse(seg); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func (n Node) Tenant() uuid.UUID {
	if n.TenantID == nil {
		return revision.Global
	}
	return *n.TenantID
}

type Service struct {
	store *store.Store
	cat   *catalog.Service
	revs  *revision.Reader
}

func New(s *store.Store, cat *catalog.Service, revs *revision.Reader) *Service {
	return &Service{store: s, cat: cat, revs: revs}
}

const nodeColumns = `id, region, tenant_id, parent_id, type, coalesce(external_id, ''), name, owner_id, path, depth, created_at, updated_at`

func scanNode(row pgx.Row) (Node, error) {
	var n Node
	err := row.Scan(&n.ID, &n.Region, &n.TenantID, &n.ParentID, &n.Type, &n.ExternalID, &n.Name, &n.OwnerID, &n.Path, &n.Depth, &n.CreatedAt, &n.UpdatedAt)
	return n, store.NotFound(err)
}

func collect(rows pgx.Rows) ([]Node, error) {
	defer rows.Close()
	var out []Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Get finds a node by id, through the global ref table.
func (t *Service) Get(ctx context.Context, id uuid.UUID) (Node, error) {
	return get(ctx, t.store.Pool, id)
}

func get(ctx context.Context, db store.DB, id uuid.UUID) (Node, error) {
	region := "default"
	if id != catalog.RootID {
		if err := db.QueryRow(ctx, `SELECT region FROM node_refs WHERE id = $1`, id).Scan(&region); err != nil {
			return Node{}, store.NotFound(err)
		}
	}
	return scanNode(db.QueryRow(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE region = $1 AND id = $2`, region, id))
}

// ByRef finds the node mirroring a service's resource, or a tenant/service by id.
func (t *Service) ByRef(ctx context.Context, typ, externalID string) (Node, error) {
	return byRef(ctx, t.store.Pool, typ, externalID)
}

func byRef(ctx context.Context, db store.DB, typ, externalID string) (Node, error) {
	var region string
	var id uuid.UUID
	if err := db.QueryRow(ctx, `SELECT region, id FROM node_refs WHERE type = $1 AND external_id = $2`, typ, externalID).Scan(&region, &id); err != nil {
		return Node{}, store.NotFound(err)
	}
	return scanNode(db.QueryRow(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE region = $1 AND id = $2`, region, id))
}

// TenantNodes is the whole tree of one tenant, in path order (parents first).
func (t *Service) TenantNodes(ctx context.Context, tenantID uuid.UUID) ([]Node, error) {
	rows, err := t.store.Pool.Query(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE tenant_id = $1 ORDER BY depth, name`, tenantID)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

// Ancestors returns the chain's nodes, root first.
func (t *Service) Ancestors(ctx context.Context, n Node) ([]Node, error) {
	rows, err := t.store.Pool.Query(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE region = ANY($1) AND id = ANY($2) ORDER BY depth`,
		[]string{n.Region, "default"}, n.Chain())
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

func (t *Service) countChildren(ctx context.Context, db store.DB, id uuid.UUID) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM nodes WHERE parent_id = $1`, id).Scan(&n)
	return n, err
}

// bump records a change in the node's tenant (or globally) and drops this
// process's cached revision so its next read sees it.
func (t *Service) bump(ctx context.Context, tx pgx.Tx, region string, tenant uuid.UUID, kind string) (int64, error) {
	rev, err := revision.Bump(ctx, tx, region, tenant, kind)
	if err == nil {
		t.revs.Forget(tenant)
	}
	return rev, err
}

func (t *Service) insertNode(ctx context.Context, tx pgx.Tx, parent Node, typ, externalID, name string, owner *uuid.UUID) (Node, error) {
	id := store.NewID()
	if externalID == "" {
		externalID = id.String()
	}
	n, err := scanNode(tx.QueryRow(ctx, `INSERT INTO nodes (region, id, tenant_id, parent_id, type, external_id, name, owner_id, path, depth)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+nodeColumns,
		parent.Region, id, parent.TenantID, parent.ID, typ, externalID, name, owner, parent.Path+id.String()+"/", parent.Depth+1))
	if err != nil {
		return Node{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO node_refs (type, external_id, id, region) VALUES ($1, $2, $3, $4)`, typ, externalID, id, parent.Region); err != nil {
		if store.IsUniqueViolation(err) {
			return Node{}, errs.Newf("a %s with id %q already exists", typ, externalID)
		}
		return Node{}, err
	}
	return n, nil
}

// ---------- Groups ----------

// CreateGroup files a new group under a tenant or another group.
func (t *Service) CreateGroup(ctx context.Context, parentID uuid.UUID, name, actor string) (Node, int64, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Node{}, 0, ErrNameMissing
	}
	snap, err := t.cat.Snapshot(ctx, false)
	if err != nil {
		return Node{}, 0, err
	}
	var n Node
	var rev int64
	err = t.store.Tx(ctx, func(tx pgx.Tx) error {
		parent, err := get(ctx, tx, parentID)
		if err != nil {
			return err
		}
		if !snap.CanParent(parent.Type, catalog.TypeGroup) {
			return ErrWrongParent
		}
		if parent.Depth+1 > MaxDepth {
			return ErrTooDeep
		}
		if n, err = t.insertNode(ctx, tx, parent, catalog.TypeGroup, "", name, nil); err != nil {
			return err
		}
		if rev, err = t.bump(ctx, tx, n.Region, n.Tenant(), "group.created"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, n.Region, actor, "group.created", "node:"+n.ID.String(), map[string]any{"name": name, "parent": parentID})
	})
	return n, rev, err
}

// Rename renames a group. Mirrored nodes are renamed by their owner module.
func (t *Service) Rename(ctx context.Context, id uuid.UUID, name, actor string) (Node, int64, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Node{}, 0, ErrNameMissing
	}
	var n Node
	var rev int64
	err := t.store.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Type != catalog.TypeGroup {
			return ErrNotGroup
		}
		if n, err = scanNode(tx.QueryRow(ctx, `UPDATE nodes SET name = $3, updated_at = now() WHERE region = $1 AND id = $2 RETURNING `+nodeColumns,
			cur.Region, id, name)); err != nil {
			return err
		}
		if rev, err = t.bump(ctx, tx, n.Region, n.Tenant(), "node.renamed"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, n.Region, actor, "node.renamed", "node:"+id.String(), map[string]any{"name": name})
	})
	return n, rev, err
}

// Move re-files a node under a new parent in the same tenant, with everything
// under it.
func (t *Service) Move(ctx context.Context, id, newParentID uuid.UUID, actor string) (Node, int64, error) {
	snap, err := t.cat.Snapshot(ctx, false)
	if err != nil {
		return Node{}, 0, err
	}
	var n Node
	var rev int64
	err = t.store.Tx(ctx, func(tx pgx.Tx) error {
		if n, err = t.move(ctx, tx, snap, id, newParentID); err != nil {
			return err
		}
		if rev, err = t.bump(ctx, tx, n.Region, n.Tenant(), "node.moved"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, n.Region, actor, "node.moved", "node:"+id.String(), map[string]any{"parent": newParentID})
	})
	return n, rev, err
}

func (t *Service) move(ctx context.Context, tx pgx.Tx, snap *catalog.Snapshot, id, newParentID uuid.UUID) (Node, error) {
	cur, err := get(ctx, tx, id)
	if err != nil {
		return Node{}, err
	}
	if cur.TenantID == nil || cur.Type == catalog.TypeTenant {
		return Node{}, ErrWrongParent
	}
	if cur.ParentID != nil && *cur.ParentID == newParentID {
		return cur, nil
	}
	parent, err := get(ctx, tx, newParentID)
	if err != nil {
		return Node{}, err
	}
	if parent.TenantID == nil || *parent.TenantID != *cur.TenantID {
		return Node{}, errs.New("a node can only move within its tenant")
	}
	if !snap.CanParent(parent.Type, cur.Type) {
		return Node{}, ErrWrongParent
	}
	if strings.HasPrefix(parent.Path, cur.Path) {
		return Node{}, ErrCycle
	}
	var deepest int
	if err := tx.QueryRow(ctx, `SELECT max(depth) FROM nodes WHERE region = $1 AND tenant_id = $2 AND path LIKE $3 || '%'`,
		cur.Region, cur.TenantID, cur.Path).Scan(&deepest); err != nil {
		return Node{}, err
	}
	delta := parent.Depth + 1 - cur.Depth
	if deepest+delta > MaxDepth {
		return Node{}, ErrTooDeep
	}
	newPath := parent.Path + cur.ID.String() + "/"
	if _, err := tx.Exec(ctx, `UPDATE nodes SET path = $4 || substr(path, length($3) + 1), depth = depth + $5, updated_at = now()
		WHERE region = $1 AND tenant_id = $2 AND path LIKE $3 || '%'`, cur.Region, cur.TenantID, cur.Path, newPath, delta); err != nil {
		return Node{}, err
	}
	return scanNode(tx.QueryRow(ctx, `UPDATE nodes SET parent_id = $3 WHERE region = $1 AND id = $2 RETURNING `+nodeColumns, cur.Region, cur.ID, parent.ID))
}

// DeleteGroup removes an empty group, its roster, the grants made on it and
// the grants made to it.
func (t *Service) DeleteGroup(ctx context.Context, id uuid.UUID, actor string) (int64, error) {
	var rev int64
	err := t.store.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.Type != catalog.TypeGroup {
			return ErrNotGroup
		}
		if rev, err = t.deleteNode(ctx, tx, cur, "group.deleted"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, cur.Region, actor, "group.deleted", "node:"+id.String(), map[string]any{"name": cur.Name})
	})
	return rev, err
}

func (t *Service) deleteNode(ctx context.Context, tx pgx.Tx, n Node, kind string) (int64, error) {
	children, err := t.countChildren(ctx, tx, n.ID)
	if err != nil {
		return 0, err
	}
	if children > 0 {
		return 0, ErrNotEmpty
	}
	for _, q := range []string{
		`DELETE FROM grants WHERE region = $1 AND node_id = $2`,
		`DELETE FROM grants WHERE region = $1 AND principal_type = 'group' AND principal_id = $2`,
		`DELETE FROM group_members WHERE region = $1 AND group_id = $2`,
		`DELETE FROM nodes WHERE region = $1 AND id = $2`,
	} {
		if _, err := tx.Exec(ctx, q, n.Region, n.ID); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM node_refs WHERE id = $1`, n.ID); err != nil {
		return 0, err
	}
	return t.bump(ctx, tx, n.Region, n.Tenant(), kind)
}

// ---------- Resources (written by the owning service) ----------

type ResourceInput struct {
	Type             string
	ExternalID       string
	Name             string
	ParentID         uuid.UUID
	ParentType       string
	ParentExternalID string
	OwnerID          *uuid.UUID
}

// UpsertResource creates or updates the node mirroring a service's resource.
// Calling it again with the same type and id is safe: it renames, re-owns or
// re-files the node as needed.
func (t *Service) UpsertResource(ctx context.Context, svc string, in ResourceInput) (Node, int64, error) {
	if catalog.ServiceOf(in.Type) != svc {
		return Node{}, 0, ErrNotYourType
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.ExternalID == "" || len(in.ExternalID) > 200 {
		return Node{}, 0, errs.New("external_id is required (up to 200 characters)")
	}
	if in.Name == "" {
		in.Name = in.ExternalID
	}
	snap, err := t.cat.Snapshot(ctx, false)
	if err != nil {
		return Node{}, 0, err
	}
	if _, ok := snap.Types[in.Type]; !ok {
		return Node{}, 0, errs.Newf("resource type %q isn't in an active contract", in.Type)
	}
	var n Node
	var rev int64
	err = t.store.Tx(ctx, func(tx pgx.Tx) error {
		var parent Node
		var err error
		if in.ParentID != uuid.Nil {
			parent, err = get(ctx, tx, in.ParentID)
		} else {
			parent, err = byRef(ctx, tx, in.ParentType, in.ParentExternalID)
		}
		if errors.Is(err, store.ErrNotFound) {
			return errs.New("the parent node doesn't exist")
		}
		if err != nil {
			return err
		}
		if parent.TenantID == nil {
			return ErrOutsideTenant
		}
		if !snap.CanParent(parent.Type, in.Type) {
			return ErrWrongParent
		}
		cur, err := byRef(ctx, tx, in.Type, in.ExternalID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			if parent.Depth+1 > MaxDepth {
				return ErrTooDeep
			}
			if n, err = t.insertNode(ctx, tx, parent, in.Type, in.ExternalID, in.Name, in.OwnerID); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if cur.ParentID == nil || *cur.ParentID != parent.ID {
				if _, err := t.move(ctx, tx, snap, cur.ID, parent.ID); err != nil {
					return err
				}
			}
			owner := cur.OwnerID
			if in.OwnerID != nil {
				owner = in.OwnerID
			}
			if n, err = scanNode(tx.QueryRow(ctx, `UPDATE nodes SET name = $3, owner_id = $4, updated_at = now()
				WHERE region = $1 AND id = $2 RETURNING `+nodeColumns, cur.Region, cur.ID, in.Name, owner)); err != nil {
				return err
			}
		}
		rev, err = t.bump(ctx, tx, n.Region, n.Tenant(), "resource.upserted")
		return err
	})
	return n, rev, err
}

// DeleteResource removes the node mirroring a service's resource, once nothing
// is filed under it.
func (t *Service) DeleteResource(ctx context.Context, svc, typ, externalID string) (int64, error) {
	if catalog.ServiceOf(typ) != svc {
		return 0, ErrNotYourType
	}
	var rev int64
	err := t.store.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := byRef(ctx, tx, typ, externalID)
		if err != nil {
			return err
		}
		rev, err = t.deleteNode(ctx, tx, cur, "resource.deleted")
		return err
	})
	return rev, err
}

// ---------- Rosters ----------

// isTenantMember reports whether a user is on the roster of the node's tenant.
func isTenantMember(ctx context.Context, db store.DB, region string, tenantID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members m JOIN nodes n ON n.region = m.region AND n.id = m.group_id
		WHERE m.region = $1 AND m.tenant_id = $2 AND m.user_id = $3 AND n.type = 'hayat/tenant')`, region, tenantID, userID).Scan(&ok)
	return ok, err
}

type Member struct {
	UserID   uuid.UUID
	JoinedAt time.Time
}

func (t *Service) GroupMembers(ctx context.Context, groupID uuid.UUID) ([]Member, error) {
	g, err := t.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	rows, err := t.store.Pool.Query(ctx, `SELECT user_id, created_at FROM group_members WHERE region = $1 AND group_id = $2 ORDER BY created_at`, g.Region, g.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddGroupMember puts a tenant member on a group's roster.
func (t *Service) AddGroupMember(ctx context.Context, groupID, userID uuid.UUID, actor string) (int64, error) {
	var rev int64
	err := t.store.Tx(ctx, func(tx pgx.Tx) error {
		g, err := get(ctx, tx, groupID)
		if err != nil {
			return err
		}
		if g.Type != catalog.TypeGroup {
			return ErrNotGroup
		}
		ok, err := isTenantMember(ctx, tx, g.Region, *g.TenantID, userID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotMember
		}
		tag, err := tx.Exec(ctx, `INSERT INTO group_members (region, group_id, tenant_id, user_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			g.Region, g.ID, g.TenantID, userID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if rev, err = t.bump(ctx, tx, g.Region, g.Tenant(), "group.member_added"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, g.Region, actor, "group.member_added", "node:"+g.ID.String(), map[string]any{"user": userID})
	})
	return rev, err
}

func (t *Service) RemoveGroupMember(ctx context.Context, groupID, userID uuid.UUID, actor string) (int64, error) {
	var rev int64
	err := t.store.Tx(ctx, func(tx pgx.Tx) error {
		g, err := get(ctx, tx, groupID)
		if err != nil {
			return err
		}
		if g.Type != catalog.TypeGroup {
			return ErrNotGroup
		}
		tag, err := tx.Exec(ctx, `DELETE FROM group_members WHERE region = $1 AND group_id = $2 AND user_id = $3`, g.Region, g.ID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return store.ErrNotFound
		}
		if rev, err = t.bump(ctx, tx, g.Region, g.Tenant(), "group.member_removed"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, g.Region, actor, "group.member_removed", "node:"+g.ID.String(), map[string]any{"user": userID})
	})
	return rev, err
}

package tree

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/store"
)

const (
	PrincipalUser  = "user"
	PrincipalGroup = "group"
)

// Grant is a role given to a principal on a node. It is what was granted
// *here*: who can actually reach a node is the authz engine's question.
type Grant struct {
	ID            uuid.UUID
	Region        string
	NodeID        uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	RoleID        uuid.UUID
	Inherit       bool
	ExpiresAt     *time.Time
	CreatedBy     string
	CreatedAt     time.Time
}

const grantColumns = `id, region, node_id, principal_type, principal_id, role_id, inherit, expires_at, created_by, created_at`

func scanGrant(row pgx.Row) (Grant, error) {
	var g Grant
	err := row.Scan(&g.ID, &g.Region, &g.NodeID, &g.PrincipalType, &g.PrincipalID, &g.RoleID, &g.Inherit, &g.ExpiresAt, &g.CreatedBy, &g.CreatedAt)
	return g, store.NotFound(err)
}

func (t *Service) Grants(ctx context.Context, nodeID uuid.UUID) ([]Grant, error) {
	n, err := t.Get(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	rows, err := t.store.Pool.Query(ctx, `SELECT `+grantColumns+` FROM grants WHERE region = $1 AND node_id = $2 ORDER BY created_at`, n.Region, n.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type GrantInput struct {
	NodeID        uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	RoleID        uuid.UUID
	Inherit       bool
	ExpiresAt     *time.Time
}

// AddGrant gives a role on a node, after checking the grant can mean what it
// says.
func (t *Service) AddGrant(ctx context.Context, in GrantInput, actor string) (Grant, int64, error) {
	snap, err := t.cat.Snapshot(ctx, true)
	if err != nil {
		return Grant{}, 0, err
	}
	role, ok := snap.Roles[in.RoleID]
	if !ok {
		return Grant{}, 0, errs.New("that role doesn't exist")
	}
	if in.ExpiresAt != nil && in.ExpiresAt.Before(time.Now()) {
		return Grant{}, 0, errs.New("the expiry is in the past")
	}
	var g Grant
	var rev int64
	err = t.store.Tx(ctx, func(tx pgx.Tx) error {
		n, err := get(ctx, tx, in.NodeID)
		if err != nil {
			return err
		}
		if !role.GrantableOn(n.Type) {
			return errs.Newf("the role %q can't be granted on a %s", role.Name, n.Type)
		}
		for _, p := range role.Permissions {
			scope := snap.Permissions[p].Scope
			if !snap.CanContain(n.Type, scope) {
				return errs.Newf("%q means nothing on a %s; grant it at the %s level or above", p, n.Type, scope)
			}
		}
		switch in.PrincipalType {
		case PrincipalUser:
			// Inside a tenant a grant to a user needs them on its roster:
			// membership gates data, so a non-member grant would leave them
			// half in. Root and service grants are instance-wide.
			if n.TenantID != nil {
				ok, err := isTenantMember(ctx, tx, n.Region, *n.TenantID, in.PrincipalID)
				if err != nil {
					return err
				}
				if !ok {
					return ErrNotMember
				}
			}
		case PrincipalGroup:
			if n.TenantID == nil {
				return errs.New("only people can be granted roles outside a tenant")
			}
			p, err := get(ctx, tx, in.PrincipalID)
			if err != nil {
				return errs.New("that group doesn't exist")
			}
			if (p.Type != catalog.TypeGroup && p.Type != catalog.TypeTenant) || p.TenantID == nil || *p.TenantID != *n.TenantID {
				return errs.New("grant to a group of this tenant, or to the tenant itself for everyone in it")
			}
		default:
			return errs.New("a principal is a user or a group")
		}
		g, err = scanGrant(tx.QueryRow(ctx, `INSERT INTO grants (region, id, tenant_id, node_id, principal_type, principal_id, role_id, inherit, expires_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (region, node_id, principal_type, principal_id, role_id) DO NOTHING RETURNING `+grantColumns,
			n.Region, store.NewID(), n.TenantID, n.ID, in.PrincipalType, in.PrincipalID, in.RoleID, in.Inherit, in.ExpiresAt, actor))
		if err == store.ErrNotFound {
			return ErrGrantExists
		}
		if err != nil {
			return err
		}
		if rev, err = t.bump(ctx, tx, n.Region, n.Tenant(), "grant.added"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, n.Region, actor, "grant.added", "node:"+n.ID.String(),
			map[string]any{"role": role.Slug, "principal": in.PrincipalType + ":" + in.PrincipalID.String(), "inherit": in.Inherit})
	})
	return g, rev, err
}

// GetGrant reads one grant, for the caller's permission check before removal.
func (t *Service) GetGrant(ctx context.Context, id uuid.UUID) (Grant, error) {
	return scanGrant(t.store.Pool.QueryRow(ctx, `SELECT `+grantColumns+` FROM grants WHERE id = $1`, id))
}

func (t *Service) RemoveGrant(ctx context.Context, id uuid.UUID, actor string) (int64, error) {
	var rev int64
	err := t.store.Tx(ctx, func(tx pgx.Tx) error {
		g, err := scanGrant(tx.QueryRow(ctx, `DELETE FROM grants WHERE id = $1 RETURNING `+grantColumns, id))
		if err != nil {
			return err
		}
		n, err := get(ctx, tx, g.NodeID)
		if err != nil {
			return err
		}
		if rev, err = t.bump(ctx, tx, n.Region, n.Tenant(), "grant.removed"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, n.Region, actor, "grant.removed", "node:"+n.ID.String(),
			map[string]any{"principal": g.PrincipalType + ":" + g.PrincipalID.String(), "role": g.RoleID})
	})
	return rev, err
}

// Package tenancy owns tenants: the organisations every service shares.
//
// A tenant is a row in `tenants` mirrored by a `hayat/tenant` node under the
// root, and its members are that node's roster in `group_members`. That is the
// shape phase 2's resource tree builds on, so membership never has to move.
//
// Rules:
//   - A tenant always has an owner (the node's owner_id), and the owner is
//     always a member. The owner can't be removed; hand ownership over first.
//   - A tenant is deleted only when nothing is filed under its node.
//   - Every membership change bumps the tenant's revision, which phase 2's
//     caches key on.
package tenancy

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

// RootID is the application root every tenant hangs off.
var RootID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

const TypeTenant = "hayat/tenant"

var (
	ErrSlugTaken   = errs.New("this slug is already used by another tenant")
	ErrBadSlug     = errs.New("a slug is 2–48 lowercase letters, digits and dashes, starting with a letter")
	ErrNotEmpty    = errs.New("this tenant still has resources filed under it; move or delete them first")
	ErrOwner       = errs.New("the owner can't be removed; transfer ownership first")
	ErrNotMember   = errs.New("the new owner must already be a member")
	ErrNameMissing = errs.New("give the tenant a name")
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,47}$`)

type Tenant struct {
	ID          uuid.UUID
	Region      string
	Slug        string
	Name        string
	OwnerID     uuid.UUID
	MemberCount int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Member struct {
	UserID   uuid.UUID
	IsOwner  bool
	JoinedAt time.Time
}

type Service struct{ store *store.Store }

func New(s *store.Store) *Service { return &Service{store: s} }

// Slugify turns a name into a slug suggestion.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteRune('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = strings.Trim(out[:48], "-")
	}
	return out
}

const tenantColumns = `t.id, t.region, t.slug, t.name, n.owner_id,
	(SELECT count(*) FROM group_members m WHERE m.region = t.region AND m.group_id = n.id),
	t.created_at, t.updated_at`

const tenantFrom = ` FROM tenants t JOIN nodes n ON n.region = t.region AND n.tenant_id = t.id AND n.type = 'hayat/tenant'`

func scanTenant(row pgx.Row) (Tenant, error) {
	var t Tenant
	var owner *uuid.UUID
	err := row.Scan(&t.ID, &t.Region, &t.Slug, &t.Name, &owner, &t.MemberCount, &t.CreatedAt, &t.UpdatedAt)
	if owner != nil {
		t.OwnerID = *owner
	}
	return t, store.NotFound(err)
}

func (s *Service) regionOf(ctx context.Context, db store.DB, id uuid.UUID) (string, error) {
	var region string
	err := db.QueryRow(ctx, `SELECT region FROM tenant_refs WHERE id = $1`, id).Scan(&region)
	return region, store.NotFound(err)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Tenant, error) {
	return s.get(ctx, s.store.Pool, id)
}

func (s *Service) get(ctx context.Context, db store.DB, id uuid.UUID) (Tenant, error) {
	region, err := s.regionOf(ctx, db, id)
	if err != nil {
		return Tenant{}, err
	}
	return scanTenant(db.QueryRow(ctx, `SELECT `+tenantColumns+tenantFrom+` WHERE t.region = $1 AND t.id = $2`, region, id))
}

func (s *Service) List(ctx context.Context, query string, before time.Time, limit int) ([]Tenant, error) {
	if before.IsZero() {
		before = time.Now().Add(time.Minute)
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.store.Pool.Query(ctx, `SELECT `+tenantColumns+tenantFrom+`
		WHERE t.created_at < $1 AND (lower(t.name) LIKE $2 OR t.slug LIKE $2)
		ORDER BY t.created_at DESC LIMIT $3`, before, pattern, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

func collect(rows pgx.Rows) ([]Tenant, error) {
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) Count(ctx context.Context) (int, error) {
	var n int
	err := s.store.Pool.QueryRow(ctx, `SELECT count(*) FROM tenants`).Scan(&n)
	return n, err
}

// ForUser lists the tenants a user belongs to.
func (s *Service) ForUser(ctx context.Context, userID uuid.UUID) ([]Tenant, error) {
	rows, err := s.store.Pool.Query(ctx, `SELECT `+tenantColumns+tenantFrom+`
		JOIN group_members gm ON gm.region = n.region AND gm.group_id = n.id
		WHERE gm.user_id = $1 ORDER BY t.name`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows)
}

// Create makes a tenant owned by ownerID, who becomes its first member. The
// tenant lives in this deployment's region.
func (s *Service) Create(ctx context.Context, name, slug string, ownerID uuid.UUID, actor string) (Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tenant{}, ErrNameMissing
	}
	if slug == "" {
		slug = Slugify(name)
	}
	if !slugRe.MatchString(slug) {
		return Tenant{}, ErrBadSlug
	}
	region := s.store.Region
	id := store.NewID()
	nodeID := store.NewID()
	var t Tenant
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_refs WHERE slug = $1)`, slug).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return ErrSlugTaken
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tenant_refs (id, slug, region) VALUES ($1, $2, $3)`, id, slug, region); err != nil {
			if store.IsUniqueViolation(err) {
				return ErrSlugTaken
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tenants (region, id, slug, name) VALUES ($1, $2, $3, $4)`, region, id, slug, name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO nodes (region, id, tenant_id, parent_id, type, external_id, name, owner_id, path, depth)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1)`,
			region, nodeID, id, RootID, TypeTenant, id.String(), name, ownerID, "/root/"+nodeID.String()+"/"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO node_refs (type, external_id, id, region) VALUES ($1, $2, $3, $4)`,
			TypeTenant, id.String(), nodeID, region); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO group_members (region, group_id, tenant_id, user_id) VALUES ($1, $2, $3, $4)`,
			region, nodeID, id, ownerID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tenant_revisions (region, tenant_id, rev) VALUES ($1, $2, 1)`, region, id); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, region, actor, "tenant.created", "tenant:"+id.String(), map[string]any{"slug": slug, "owner": ownerID}); err != nil {
			return err
		}
		var err error
		t, err = s.get(ctx, tx, id)
		return err
	})
	return t, err
}

func (s *Service) Rename(ctx context.Context, id uuid.UUID, name, actor string) (Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tenant{}, ErrNameMissing
	}
	var t Tenant
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tenants SET name = $3, updated_at = now() WHERE region = $1 AND id = $2`, region, id, name); err != nil {
			return err
		}
		// The node mirrors the tenant, so it is renamed with it.
		if _, err := tx.Exec(ctx, `UPDATE nodes SET name = $3, updated_at = now() WHERE region = $1 AND tenant_id = $2 AND type = $4`,
			region, id, name, TypeTenant); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, region, actor, "tenant.renamed", "tenant:"+id.String(), map[string]any{"name": name}); err != nil {
			return err
		}
		t, err = s.get(ctx, tx, id)
		return err
	})
	return t, err
}

// Delete removes an empty tenant and its roster.
func (s *Service) Delete(ctx context.Context, id uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, nodeID, err := s.node(ctx, tx, id)
		if err != nil {
			return err
		}
		var children int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM nodes WHERE parent_id = $1`, nodeID).Scan(&children); err != nil {
			return err
		}
		if children > 0 {
			return ErrNotEmpty
		}
		for _, q := range []string{
			`DELETE FROM group_members WHERE region = $1 AND group_id = $2`,
			`DELETE FROM grants WHERE region = $1 AND (node_id = $2 OR principal_id = $2)`,
			`DELETE FROM nodes WHERE region = $1 AND id = $2`,
		} {
			if _, err := tx.Exec(ctx, q, region, nodeID); err != nil {
				return err
			}
		}
		for _, q := range []string{
			`DELETE FROM tenant_revisions WHERE region = $1 AND tenant_id = $2`,
			`DELETE FROM tenants WHERE region = $1 AND id = $2`,
		} {
			if _, err := tx.Exec(ctx, q, region, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM node_refs WHERE type = $1 AND external_id = $2`, TypeTenant, id.String()); err != nil {
			return err
		}
		if _, err := revision.Bump(ctx, tx, region, revision.Global, "tenant.deleted"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM tenant_refs WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "tenant.deleted", "tenant:"+id.String(), nil)
	})
}

func (s *Service) node(ctx context.Context, db store.DB, tenantID uuid.UUID) (string, uuid.UUID, error) {
	region, err := s.regionOf(ctx, db, tenantID)
	if err != nil {
		return "", uuid.Nil, err
	}
	var nodeID uuid.UUID
	err = db.QueryRow(ctx, `SELECT id FROM nodes WHERE region = $1 AND tenant_id = $2 AND type = $3`, region, tenantID, TypeTenant).Scan(&nodeID)
	return region, nodeID, store.NotFound(err)
}

func bump(ctx context.Context, tx pgx.Tx, region string, tenantID uuid.UUID) error {
	_, err := revision.Bump(ctx, tx, region, tenantID, "tenant.roster")
	return err
}

// ---------- Members ----------

func (s *Service) Members(ctx context.Context, tenantID uuid.UUID) ([]Member, error) {
	region, nodeID, err := s.node(ctx, s.store.Pool, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.Pool.Query(ctx, `SELECT m.user_id, m.user_id = n.owner_id, m.created_at
		FROM group_members m JOIN nodes n ON n.region = m.region AND n.id = m.group_id
		WHERE m.region = $1 AND m.group_id = $2 ORDER BY m.created_at`, region, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.IsOwner, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) IsMember(ctx context.Context, tenantID, userID uuid.UUID) (bool, error) {
	region, nodeID, err := s.node(ctx, s.store.Pool, tenantID)
	if err != nil {
		return false, err
	}
	var ok bool
	err = s.store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE region = $1 AND group_id = $2 AND user_id = $3)`,
		region, nodeID, userID).Scan(&ok)
	return ok, err
}

// AddMember puts a user on the tenant's roster. Adding someone already there
// is not an error.
func (s *Service) AddMember(ctx context.Context, tenantID, userID uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, nodeID, err := s.node(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO group_members (region, group_id, tenant_id, user_id) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, region, nodeID, tenantID, userID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if err := bump(ctx, tx, region, tenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "tenant.member_added", "tenant:"+tenantID.String(), map[string]any{"user": userID})
	})
}

func (s *Service) RemoveMember(ctx context.Context, tenantID, userID uuid.UUID, actor string) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		t, err := s.get(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		if t.OwnerID == userID {
			return ErrOwner
		}
		_, nodeID, err := s.node(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM group_members WHERE region = $1 AND group_id = $2 AND user_id = $3`, t.Region, nodeID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return store.ErrNotFound
		}
		// Leaving the tenant takes everything that came with being in it: the
		// group rosters and the grants made to them inside it.
		if _, err := tx.Exec(ctx, `DELETE FROM group_members WHERE region = $1 AND tenant_id = $2 AND user_id = $3`, t.Region, tenantID, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM grants WHERE region = $1 AND tenant_id = $2 AND principal_type = 'user' AND principal_id = $3`,
			t.Region, tenantID, userID); err != nil {
			return err
		}
		if err := bump(ctx, tx, t.Region, tenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, t.Region, actor, "tenant.member_removed", "tenant:"+tenantID.String(), map[string]any{"user": userID})
	})
}

// TransferOwnership hands the tenant to another member.
func (s *Service) TransferOwnership(ctx context.Context, tenantID, userID uuid.UUID, actor string) (Tenant, error) {
	var t Tenant
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, nodeID, err := s.node(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE region = $1 AND group_id = $2 AND user_id = $3)`,
			region, nodeID, userID).Scan(&member); err != nil {
			return err
		}
		if !member {
			return ErrNotMember
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET owner_id = $3, updated_at = now() WHERE region = $1 AND id = $2`, region, nodeID, userID); err != nil {
			return err
		}
		if err := bump(ctx, tx, region, tenantID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, region, actor, "tenant.ownership_transferred", "tenant:"+tenantID.String(), map[string]any{"owner": userID}); err != nil {
			return err
		}
		t, err = s.get(ctx, tx, tenantID)
		return err
	})
	return t, err
}

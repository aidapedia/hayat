package catalog

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

var (
	ErrRoleInUse         = errs.New("this role is still granted somewhere; remove those grants first")
	ErrRoleOwned         = errs.New("this role comes from a service contract; change it by publishing a new contract version")
	ErrRoleBuiltin       = errs.New("built-in roles can't be deleted")
	ErrRoleNameMissing   = errs.New("give the role a name")
	ErrUnknownPermission = errs.New("one of the permissions doesn't exist")
)

type RoleInput struct {
	Name        string
	Description string
	AppliesTo   []string
	Permissions []string
}

func (c *Service) Roles(ctx context.Context) ([]Role, error) {
	snap, err := c.Snapshot(ctx, true)
	if err != nil {
		return nil, err
	}
	roles, err := listRoles(ctx, c.store.Pool)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		roles[i].Permissions = slices.DeleteFunc(roles[i].Permissions, func(p string) bool { return !snap.Known(p) })
	}
	return roles, nil
}

// RoleUsage counts grants per role.
func (c *Service) RoleUsage(ctx context.Context) (map[uuid.UUID]int, error) {
	rows, err := c.store.Pool.Query(ctx, `SELECT role_id, count(*) FROM grants GROUP BY role_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]int{}
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (c *Service) validateRole(ctx context.Context, in *RoleInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		return ErrRoleNameMissing
	}
	snap, err := c.Snapshot(ctx, true)
	if err != nil {
		return err
	}
	for _, p := range in.Permissions {
		if !snap.Known(p) {
			return errs.Newf("permission %q doesn't exist", p)
		}
	}
	for _, t := range in.AppliesTo {
		if _, ok := snap.Types[t]; !ok {
			return errs.Newf("node type %q doesn't exist", t)
		}
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// CreateRole makes a role an admin owns.
func (c *Service) CreateRole(ctx context.Context, in RoleInput, actor string) (Role, error) {
	if err := c.validateRole(ctx, &in); err != nil {
		return Role{}, err
	}
	base := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(in.Name), "-"), "-")
	if base == "" {
		base = "role"
	}
	id := store.NewID()
	err := c.store.Tx(ctx, func(tx pgx.Tx) error {
		slug := base
		for i := 2; ; i++ {
			var taken bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE slug = $1)`, slug).Scan(&taken); err != nil {
				return err
			}
			if !taken {
				break
			}
			slug = base + "-" + itoa(i)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO roles (id, slug, name, description, applies_to) VALUES ($1, $2, $3, $4, $5)`,
			id, slug, in.Name, strings.TrimSpace(in.Description), nonNil(in.AppliesTo)); err != nil {
			return err
		}
		if err := setRolePermissions(ctx, tx, id, in.Permissions); err != nil {
			return err
		}
		if _, err := revision.Bump(ctx, tx, c.store.Region, revision.Global, "role.created"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, c.store.Region, actor, "role.created", "role:"+slug, map[string]any{"permissions": in.Permissions})
	})
	if err != nil {
		return Role{}, err
	}
	c.revs.Forget(revision.Global)
	return c.role(ctx, id)
}

// UpdateRole changes a role. A service's contract roles are the contract's;
// built-in roles may change, which is how an instance tunes what they grant.
func (c *Service) UpdateRole(ctx context.Context, id uuid.UUID, in RoleInput, actor string) (Role, error) {
	cur, err := c.role(ctx, id)
	if err != nil {
		return Role{}, err
	}
	if cur.Service != "" && cur.Service != Platform {
		return Role{}, ErrRoleOwned
	}
	if err := c.validateRole(ctx, &in); err != nil {
		return Role{}, err
	}
	err = c.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE roles SET name = $2, description = $3, applies_to = $4, updated_at = now() WHERE id = $1`,
			id, in.Name, strings.TrimSpace(in.Description), nonNil(in.AppliesTo)); err != nil {
			return err
		}
		if err := setRolePermissions(ctx, tx, id, in.Permissions); err != nil {
			return err
		}
		if _, err := revision.Bump(ctx, tx, c.store.Region, revision.Global, "role.updated"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, c.store.Region, actor, "role.updated", "role:"+cur.Slug, map[string]any{"permissions": in.Permissions})
	})
	if err != nil {
		return Role{}, err
	}
	c.revs.Forget(revision.Global)
	return c.role(ctx, id)
}

func (c *Service) DeleteRole(ctx context.Context, id uuid.UUID, actor string) error {
	cur, err := c.role(ctx, id)
	if err != nil {
		return err
	}
	if IsBuiltinRole(id) {
		return ErrRoleBuiltin
	}
	if cur.Service != "" {
		return ErrRoleOwned
	}
	err = c.store.Tx(ctx, func(tx pgx.Tx) error {
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM grants WHERE role_id = $1)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return ErrRoleInUse
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id); err != nil {
			return err
		}
		if _, err := revision.Bump(ctx, tx, c.store.Region, revision.Global, "role.deleted"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, c.store.Region, actor, "role.deleted", "role:"+cur.Slug, nil)
	})
	if err == nil {
		c.revs.Forget(revision.Global)
	}
	return err
}

func (c *Service) role(ctx context.Context, id uuid.UUID) (Role, error) {
	roles, err := listRoles(ctx, c.store.Pool)
	if err != nil {
		return Role{}, err
	}
	for _, r := range roles {
		if r.ID == id {
			return r, nil
		}
	}
	return Role{}, store.ErrNotFound
}

func setRolePermissions(ctx context.Context, tx pgx.Tx, id uuid.UUID, perms []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, id); err != nil {
		return err
	}
	for _, p := range perms {
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, p); err != nil {
			return err
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

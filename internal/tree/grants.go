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

// RoleGrants lists where a role is granted, across every tenant and region,
// newest first, at most limit of them. It answers "who has this role" for an
// admin, and why a role can't be deleted yet.
func (t *Service) RoleGrants(ctx context.Context, roleID uuid.UUID, limit int) ([]Grant, error) {
	rows, err := t.store.Pool.Query(ctx, `SELECT `+grantColumns+` FROM grants WHERE role_id = $1 ORDER BY created_at DESC LIMIT $2`, roleID, limit)
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

// Ways a MemberGrant reaches its member.
const (
	ViaOwner    = "owner"    // they own the node
	ViaUser     = "user"     // granted to them
	ViaGroup    = "group"    // granted to a group they are on
	ViaEveryone = "everyone" // granted to the tenant: everyone on its roster
)

// MemberGrant is something a tenant member was given: a grant that reaches
// them, or a node they own (Via == ViaOwner, no role).
type MemberGrant struct {
	UserID    uuid.UUID
	Node      Node
	Via       string
	RoleSlug  string
	RoleName  string
	Inherit   bool
	GroupID   uuid.UUID
	GroupName string
}

// MemberGrants lists what the tenant's members were given, the way the authz
// engine reads it: their own grants, their groups' (a group they are on, or one
// it is nested under), the tenant's "everyone" grants (the roster is the
// tenant node's group_members), and the nodes they own. Expired grants are left out. `userIDs` narrows it to
// those members; `keep` filters by node type.
func (t *Service) MemberGrants(ctx context.Context, tenantID uuid.UUID, userIDs []uuid.UUID, keep func(nodeType string) bool) ([]MemberGrant, error) {
	tenant, err := t.ByRef(ctx, catalog.TypeTenant, tenantID.String())
	if err != nil {
		return nil, err
	}
	if userIDs == nil {
		userIDs = []uuid.UUID{}
	}
	// nodeColumns, read through the alias n.
	const cols = `n.id, n.region, n.tenant_id, n.parent_id, n.type, coalesce(n.external_id, ''), n.name, n.owner_id, n.path, n.depth, n.created_at, n.updated_at`
	rows, err := t.store.Pool.Query(ctx, `
		WITH roster AS (
			SELECT user_id FROM group_members
			WHERE region = $1 AND group_id = $2 AND (cardinality($4::uuid[]) = 0 OR user_id = ANY($4))
		), reach AS (
			SELECT r.user_id, 'user' AS principal_type, r.user_id AS principal_id FROM roster r
			UNION ALL
			SELECT DISTINCT gm.user_id, 'group', a.id FROM group_members gm JOIN roster r ON r.user_id = gm.user_id
			JOIN nodes g ON g.region = gm.region AND g.id = gm.group_id
			JOIN nodes a ON a.region = g.region AND a.tenant_id = g.tenant_id
				AND (a.id = g.id OR (a.type = 'hayat/group' AND g.type = 'hayat/group' AND g.path LIKE a.path || '%'))
			WHERE gm.region = $1 AND gm.tenant_id = $3
		)
		SELECT r.user_id, `+cols+`, r.principal_type, r.principal_id, ro.slug, ro.name, g.inherit, coalesce(p.name, '')
		FROM reach r
		JOIN grants g ON g.region = $1 AND g.principal_type = r.principal_type AND g.principal_id = r.principal_id
		JOIN nodes n ON n.region = g.region AND n.id = g.node_id
		JOIN roles ro ON ro.id = g.role_id
		LEFT JOIN nodes p ON r.principal_type = 'group' AND p.region = $1 AND p.id = r.principal_id
		WHERE n.tenant_id = $3 AND (g.expires_at IS NULL OR g.expires_at > now())
		UNION ALL
		SELECT r.user_id, `+cols+`, 'owner', r.user_id, '', '', true, ''
		FROM roster r JOIN nodes n ON n.region = $1 AND n.tenant_id = $3 AND n.owner_id = r.user_id
		ORDER BY 1, 11, 8`, tenant.Region, tenant.ID, tenantID, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemberGrant{}
	for rows.Next() {
		var m MemberGrant
		var principalType string
		var principalID uuid.UUID
		n := &m.Node
		if err := rows.Scan(&m.UserID, &n.ID, &n.Region, &n.TenantID, &n.ParentID, &n.Type, &n.ExternalID, &n.Name, &n.OwnerID,
			&n.Path, &n.Depth, &n.CreatedAt, &n.UpdatedAt, &principalType, &principalID, &m.RoleSlug, &m.RoleName, &m.Inherit, &m.GroupName); err != nil {
			return nil, err
		}
		if keep != nil && !keep(n.Type) {
			continue
		}
		switch {
		case principalType == ViaOwner:
			m.Via, m.GroupName = ViaOwner, ""
		case principalType == PrincipalUser:
			m.Via = ViaUser
		case principalID == tenant.ID:
			m.Via, m.GroupName = ViaEveryone, ""
		default:
			m.Via, m.GroupID = ViaGroup, principalID
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ViaPlacement: the node is filed under a group whose roster they are on, and
// its type opens by placement.
const ViaPlacement = "placement"

// Access is one way a person reaches a node.
type Access struct {
	UserID uuid.UUID
	Via    string // owner | user | group | everyone | placement
	// Source is where it comes from: the node they own, the node the grant
	// was made on, or the group the node is filed under.
	Source    Node
	RoleID    uuid.UUID // a grant's role; zero for owner and placement
	RoleName  string
	Inherit   bool
	ExpiresAt *time.Time
	GroupID   uuid.UUID // via group
	GroupName string
	// Through is the group nested under GroupID they are on, when they aren't
	// on GroupID's own roster.
	ThroughID   uuid.UUID
	ThroughName string
	// Permission is what placement gives, the type's access permission.
	Permission string
}

// NodeAccess lists everyone who reaches n and every way they do, read the
// way the authz engine walks the chain: owners of the node or any node above
// it, grants on the node or inherited from above (a group's expanded to its
// roster and the rosters of the groups nested under it; the tenant's to its
// roster), and placement when the type opens that way: the direct rosters of
// the groups above, and the groups nested under the nearest of them. A type that requires membership leaves out
// whoever isn't on the tenant's roster. Expired grants and roles the catalog
// no longer has are left out. What was given, not what it adds up to.
func (t *Service) NodeAccess(ctx context.Context, n Node) ([]Access, error) {
	snap, err := t.cat.Snapshot(ctx, false)
	if err != nil {
		return nil, err
	}
	chain, err := t.Ancestors(ctx, n)
	if err != nil {
		return nil, err
	}
	db := t.store.Pool
	regions := []string{n.Region, "default"}

	var out []Access
	for _, anc := range chain {
		if anc.OwnerID != nil {
			out = append(out, Access{UserID: *anc.OwnerID, Via: ViaOwner, Source: anc})
		}
	}

	type grantRow struct {
		nodeID, principalID, roleID uuid.UUID
		principalType               string
		inherit                     bool
		expires                     *time.Time
	}
	rows, err := db.Query(ctx, `SELECT node_id, principal_type, principal_id, role_id, inherit, expires_at FROM grants
		WHERE region = ANY($1) AND node_id = ANY($2) AND (node_id = $3 OR inherit) AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at`, regions, n.Chain(), n.ID)
	if err != nil {
		return nil, err
	}
	var grants []grantRow
	for rows.Next() {
		var g grantRow
		if err := rows.Scan(&g.nodeID, &g.principalType, &g.principalID, &g.roleID, &g.inherit, &g.expires); err != nil {
			rows.Close()
			return nil, err
		}
		grants = append(grants, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Rosters of the groups granted to and of the groups above (placement).
	groupIDs := []uuid.UUID{}
	for _, g := range grants {
		if g.principalType == PrincipalGroup {
			groupIDs = append(groupIDs, g.principalID)
		}
	}
	nt := snap.Types[n.Type]
	placement := nt.ViaPlacement && nt.AccessPermission != ""
	var tenant uuid.UUID // the tenant's node, whose roster is the membership gate
	for _, anc := range chain {
		if placement && anc.ID != n.ID && anc.Type == catalog.TypeGroup {
			groupIDs = append(groupIDs, anc.ID)
		}
		if anc.Type == catalog.TypeTenant {
			tenant = anc.ID
			groupIDs = append(groupIDs, anc.ID)
		}
	}
	groups := map[uuid.UUID]Node{}
	rosters := map[uuid.UUID][]uuid.UUID{}
	if len(groupIDs) > 0 {
		rows, err := db.Query(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE region = ANY($1) AND id = ANY($2)`, regions, groupIDs)
		if err != nil {
			return nil, err
		}
		list, err := collect(rows)
		if err != nil {
			return nil, err
		}
		for _, g := range list {
			groups[g.ID] = g
		}
		rows, err = db.Query(ctx, `SELECT group_id, user_id FROM group_members WHERE region = ANY($1) AND group_id = ANY($2) ORDER BY created_at`, regions, groupIDs)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var g, u uuid.UUID
			if err := rows.Scan(&g, &u); err != nil {
				rows.Close()
				return nil, err
			}
			rosters[g] = append(rosters[g], u)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Who counts in a group through one nested under it: the granted groups,
	// and the nearest group above for placement.
	var nearest Node
	if placement {
		for i := len(chain) - 1; i >= 0; i-- {
			if chain[i].ID != n.ID && chain[i].Type == catalog.TypeGroup {
				nearest = chain[i]
				break
			}
		}
	}
	nestIn := []uuid.UUID{}
	for id, g := range groups {
		if g.Type == catalog.TypeGroup {
			nestIn = append(nestIn, id)
		}
	}
	nested, err := nestedRosters(ctx, db, regions, nestIn)
	if err != nil {
		return nil, err
	}

	byID := map[uuid.UUID]Node{}
	for _, anc := range chain {
		byID[anc.ID] = anc
	}
	for _, g := range grants {
		role, ok := snap.Roles[g.roleID]
		if !ok {
			continue
		}
		a := Access{Source: byID[g.nodeID], RoleID: role.ID, RoleName: role.Name, Inherit: g.inherit, ExpiresAt: g.expires}
		if g.principalType == PrincipalUser {
			a.UserID, a.Via = g.principalID, ViaUser
			out = append(out, a)
			continue
		}
		p, ok := groups[g.principalID]
		if !ok {
			continue
		}
		if p.Type == catalog.TypeTenant {
			a.Via = ViaEveryone
		} else {
			a.Via, a.GroupID, a.GroupName = ViaGroup, p.ID, p.Name
		}
		for _, u := range rosters[p.ID] {
			a.UserID = u
			out = append(out, a)
		}
		for _, nm := range nested[p.ID] {
			a.UserID, a.ThroughID, a.ThroughName = nm.userID, nm.via.ID, nm.via.Name
			out = append(out, a)
		}
	}
	if placement {
		for _, anc := range chain {
			if anc.ID == n.ID || anc.Type != catalog.TypeGroup {
				continue
			}
			for _, u := range rosters[anc.ID] {
				out = append(out, Access{UserID: u, Via: ViaPlacement, Source: anc, GroupID: anc.ID, GroupName: anc.Name, Permission: nt.AccessPermission})
			}
		}
		if nearest.ID != uuid.Nil {
			for _, nm := range nested[nearest.ID] {
				out = append(out, Access{UserID: nm.userID, Via: ViaPlacement, Source: nearest, GroupID: nearest.ID, GroupName: nearest.Name,
					ThroughID: nm.via.ID, ThroughName: nm.via.Name, Permission: nt.AccessPermission})
			}
		}
	}

	if nt.RequiresMembership && tenant != uuid.Nil {
		member := map[uuid.UUID]bool{}
		for _, u := range rosters[tenant] {
			member[u] = true
		}
		kept := out[:0]
		for _, a := range out {
			if member[a.UserID] {
				kept = append(kept, a)
			}
		}
		out = kept
	}
	return out, nil
}

type nestedMember struct {
	userID uuid.UUID
	via    Node // the group closest under the outer one that they are on
}

// nestedRosters lists, for each of these groups, the people who count in it
// only through a group nested under it: one entry each, through the
// shallowest such group.
func nestedRosters(ctx context.Context, db store.DB, regions []string, groupIDs []uuid.UUID) (map[uuid.UUID][]nestedMember, error) {
	out := map[uuid.UUID][]nestedMember{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT p.id, m.user_id, `+prefixed("n")+`
		FROM nodes p
		JOIN nodes n ON n.region = p.region AND n.tenant_id = p.tenant_id AND n.type = 'hayat/group' AND n.id <> p.id AND n.path LIKE p.path || '%'
		JOIN group_members m ON m.region = n.region AND m.group_id = n.id
		WHERE p.region = ANY($1) AND p.id = ANY($2)
		  AND NOT EXISTS (SELECT 1 FROM group_members d WHERE d.region = p.region AND d.group_id = p.id AND d.user_id = m.user_id)
		ORDER BY p.id, n.depth, n.name`, regions, groupIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[[2]uuid.UUID]bool{}
	for rows.Next() {
		var outer, u uuid.UUID
		var g Node
		if err := rows.Scan(&outer, &u, &g.ID, &g.Region, &g.TenantID, &g.ParentID, &g.Type, &g.ExternalID, &g.Name, &g.OwnerID,
			&g.Path, &g.Depth, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		if key := [2]uuid.UUID{outer, u}; !seen[key] {
			seen[key] = true
			out[outer] = append(out[outer], nestedMember{userID: u, via: g})
		}
	}
	return out, rows.Err()
}

// Package authz decides. Given a person and a node, it works out everything
// they may do there, and why.
//
// The rule, walking the chain from the root down to the node:
//  1. Owning any node on the chain allows every permission.
//  2. A grant counts on its own node always, and below it only when it
//     inherits. It counts when it names the person, or a group (or the tenant)
//     whose roster holds them. Expired grants don't count.
//  3. Placement: when the node's type grants its access permission by
//     placement, being on the roster of a hayat/group above it gives that
//     permission. The tenant never counts here — its roster is everyone, and
//     counting it would make every access list dead letter.
//  4. A type that requires membership allows nothing to someone who isn't on
//     the tenant's roster, whatever was granted.
//  5. Instance admins hold Hayat's own platform permissions everywhere. Never a
//     service's: administering the instance is not reading its data.
//  6. Keys the catalog doesn't know are dropped, never honoured.
//
// Union only, no deny rules: that keeps decisions cacheable and explainable.
// Results are cached under (global revision, tenant revision, person, node),
// so any policy change in the tenant makes new keys and old entries die.
package authz

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tree"
)

const (
	ReasonOwner     = "owner"
	ReasonGrant     = "grant"
	ReasonPlacement = "placement"
	ReasonAdmin     = "admin"
)

// Reason is why a permission is held: the first thing on the chain that gave it.
type Reason struct {
	Kind          string
	NodeID        uuid.UUID
	NodeName      string
	NodeType      string
	RoleID        uuid.UUID
	RoleName      string
	PrincipalType string
	PrincipalID   uuid.UUID
	PrincipalName string
}

func (r Reason) String() string {
	switch r.Kind {
	case ReasonOwner:
		return fmt.Sprintf("owns %s %q", kindName(r.NodeType), r.NodeName)
	case ReasonGrant:
		via := "granted to them directly"
		if r.PrincipalType == tree.PrincipalGroup {
			via = fmt.Sprintf("granted to %s", r.PrincipalName)
		}
		return fmt.Sprintf("%s role on %s %q, %s", r.RoleName, kindName(r.NodeType), r.NodeName, via)
	case ReasonPlacement:
		return fmt.Sprintf("on the roster of group %q, where this is filed", r.NodeName)
	case ReasonAdmin:
		return "instance admin"
	}
	return r.Kind
}

// kindName is how a node type reads in a sentence.
func kindName(typ string) string {
	switch typ {
	case catalog.TypeTenant:
		return "tenant"
	case catalog.TypeGroup:
		return "group"
	case catalog.TypeRoot:
		return "the instance"
	}
	return typ
}

// Result is everything a person may do at one node.
type Result struct {
	Perms     map[string]Reason
	TenantID  uuid.UUID
	Rev       int64 // the tenant's revision the answer was computed at
	GlobalRev int64
	Member    bool
	until     time.Time
}

func (r *Result) Has(perm string) bool { _, ok := r.Perms[perm]; return ok }

// Consistency says how fresh an answer must be.
type Consistency struct {
	// AtLeastRev re-reads the tenant revision if the cached one is older:
	// read-your-writes after a change.
	AtLeastRev int64
	// Fresh bypasses every cache: for destructive actions.
	Fresh bool
}

type Engine struct {
	store *store.Store
	cat   *catalog.Service
	ids   *identity.Service
	revs  *revision.Reader
	cache *lru.LRU[string, *Result]
}

func New(s *store.Store, cat *catalog.Service, ids *identity.Service, revs *revision.Reader) *Engine {
	return &Engine{store: s, cat: cat, ids: ids, revs: revs, cache: lru.NewLRU[string, *Result](100_000, nil, 5*time.Minute)}
}

// At returns everything userID may do at node.
func (e *Engine) At(ctx context.Context, userID uuid.UUID, node tree.Node, c Consistency) (*Result, error) {
	global, err := e.revs.Get(ctx, revision.Global, 0, c.Fresh)
	if err != nil {
		return nil, err
	}
	tenantRev := global
	if node.TenantID != nil {
		if tenantRev, err = e.revs.Get(ctx, *node.TenantID, c.AtLeastRev, c.Fresh); err != nil {
			return nil, err
		}
	}
	key := fmt.Sprintf("%d:%d:%s:%s", global, tenantRev, userID, node.ID)
	if !c.Fresh {
		if r, ok := e.cache.Get(key); ok && time.Now().Before(r.until) {
			return r, nil
		}
	}
	r, err := e.compute(ctx, userID, node, c.Fresh)
	if err != nil {
		return nil, err
	}
	r.Rev, r.GlobalRev = tenantRev, global
	e.cache.Add(key, r)
	return r, nil
}

func (e *Engine) compute(ctx context.Context, userID uuid.UUID, node tree.Node, fresh bool) (*Result, error) {
	r := &Result{Perms: map[string]Reason{}, TenantID: node.Tenant(), until: time.Now().Add(5 * time.Minute)}
	snap, err := e.cat.Snapshot(ctx, fresh)
	if err != nil {
		return nil, err
	}
	u, err := e.ids.GetUser(ctx, userID)
	if err != nil || u.Status != identity.StatusActive {
		return r, nil // unknown or inactive: nothing
	}

	db := e.store.Pool
	regions := []string{node.Region, "default"}
	chainIDs := node.Chain()
	chain, err := loadNodes(ctx, db, regions, chainIDs)
	if err != nil {
		return nil, err
	}

	groups := map[uuid.UUID]bool{}
	if node.TenantID != nil {
		rows, err := db.Query(ctx, `SELECT group_id FROM group_members WHERE region = $1 AND tenant_id = $2 AND user_id = $3`,
			node.Region, *node.TenantID, userID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var g uuid.UUID
			if err := rows.Scan(&g); err == nil {
				groups[g] = true
			}
		}
		rows.Close()
		for _, n := range chain {
			if n.Type == catalog.TypeTenant && groups[n.ID] {
				r.Member = true
			}
		}
	}

	type grantRow struct {
		nodeID, principalID, roleID uuid.UUID
		principalType               string
		inherit                     bool
		expires                     *time.Time
	}
	rows, err := db.Query(ctx, `SELECT node_id, principal_type, principal_id, role_id, inherit, expires_at FROM grants
		WHERE region = ANY($1) AND node_id = ANY($2) AND (expires_at IS NULL OR expires_at > now())`, regions, chainIDs)
	if err != nil {
		return nil, err
	}
	var grants []grantRow
	for rows.Next() {
		var g grantRow
		if err := rows.Scan(&g.nodeID, &g.principalType, &g.principalID, &g.roleID, &g.inherit, &g.expires); err == nil {
			grants = append(grants, g)
		}
	}
	rows.Close()

	// Names of the groups (or tenant) grants were made to, for the reasons.
	groupNames := map[uuid.UUID]string{}
	var groupPrincipals []uuid.UUID
	for _, g := range grants {
		if g.principalType == tree.PrincipalGroup && groups[g.principalID] {
			groupPrincipals = append(groupPrincipals, g.principalID)
		}
	}
	if len(groupPrincipals) > 0 {
		rows, err := db.Query(ctx, `SELECT id, name, type FROM nodes WHERE region = ANY($1) AND id = ANY($2)`, regions, groupPrincipals)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id uuid.UUID
			var name, typ string
			if rows.Scan(&id, &name, &typ) == nil {
				if typ == catalog.TypeTenant {
					groupNames[id] = fmt.Sprintf("everyone in %q", name)
				} else {
					groupNames[id] = fmt.Sprintf("group %q", name)
				}
			}
		}
		rows.Close()
	}

	add := func(perm string, why Reason) {
		if _, held := r.Perms[perm]; !held && snap.Known(perm) {
			r.Perms[perm] = why
		}
	}
	at := func(n tree.Node) Reason { return Reason{NodeID: n.ID, NodeName: n.Name, NodeType: n.Type} }

	for _, anc := range chain {
		if anc.OwnerID != nil && *anc.OwnerID == userID {
			why := at(anc)
			why.Kind = ReasonOwner
			for p := range snap.Permissions {
				add(p, why)
			}
			break
		}
		here := anc.ID == node.ID
		for _, g := range grants {
			if g.nodeID != anc.ID || (!here && !g.inherit) {
				continue
			}
			mine := (g.principalType == tree.PrincipalUser && g.principalID == userID) ||
				(g.principalType == tree.PrincipalGroup && groups[g.principalID])
			if !mine {
				continue
			}
			role, ok := snap.Roles[g.roleID]
			if !ok {
				continue
			}
			if g.expires != nil && g.expires.Before(r.until) {
				r.until = *g.expires // the answer stops being true when the grant expires
			}
			why := at(anc)
			why.Kind, why.RoleID, why.RoleName, why.PrincipalType, why.PrincipalID = ReasonGrant, role.ID, role.Name, g.principalType, g.principalID
			why.PrincipalName = groupNames[g.principalID]
			for _, p := range role.Permissions {
				add(p, why)
			}
		}
	}

	nt := snap.Types[node.Type]
	if nt.ViaPlacement && nt.AccessPermission != "" {
		for _, anc := range chain {
			if anc.ID != node.ID && anc.Type == catalog.TypeGroup && groups[anc.ID] {
				why := at(anc)
				why.Kind = ReasonPlacement
				add(nt.AccessPermission, why)
				break
			}
		}
	}

	if nt.RequiresMembership && !r.Member {
		r.Perms = map[string]Reason{}
	}

	if u.IsAdmin() {
		for p, perm := range snap.Permissions {
			if perm.Service == catalog.Platform {
				add(p, Reason{Kind: ReasonAdmin})
			}
		}
	}
	return r, nil
}

func loadNodes(ctx context.Context, db store.DB, regions []string, ids []uuid.UUID) ([]tree.Node, error) {
	rows, err := db.Query(ctx, `SELECT id, region, tenant_id, parent_id, type, coalesce(external_id, ''), name, owner_id, path, depth, created_at, updated_at
		FROM nodes WHERE region = ANY($1) AND id = ANY($2) ORDER BY depth`, regions, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tree.Node
	for rows.Next() {
		var n tree.Node
		if err := rows.Scan(&n.ID, &n.Region, &n.TenantID, &n.ParentID, &n.Type, &n.ExternalID, &n.Name, &n.OwnerID, &n.Path, &n.Depth, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Check answers one question.
func (e *Engine) Check(ctx context.Context, userID uuid.UUID, perm string, node tree.Node, c Consistency) (bool, Reason, *Result, error) {
	r, err := e.At(ctx, userID, node, c)
	if err != nil {
		return false, Reason{}, nil, err
	}
	why, ok := r.Perms[perm]
	return ok, why, r, nil
}

// ---------- Lookups ----------

// LookupResources lists the nodes of one type where userID holds perm,
// optionally inside one tenant. Candidates come from the grants, ownership
// and placement that could apply; each is then confirmed with At, so the
// list never disagrees with Check.
func (e *Engine) LookupResources(ctx context.Context, userID uuid.UUID, perm, typ string, tenantID *uuid.UUID, c Consistency, limit int) ([]tree.Node, error) {
	snap, err := e.cat.Snapshot(ctx, c.Fresh)
	if err != nil {
		return nil, err
	}
	if _, ok := snap.Types[typ]; !ok || !snap.Known(perm) {
		return nil, nil
	}
	db := e.store.Pool

	var groupIDs []uuid.UUID
	q := `SELECT group_id FROM group_members WHERE user_id = $1`
	args := []any{userID}
	if tenantID != nil {
		q += ` AND tenant_id = $2`
		args = append(args, *tenantID)
	}
	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g uuid.UUID
		if rows.Scan(&g) == nil {
			groupIDs = append(groupIDs, g)
		}
	}
	rows.Close()

	seeds := map[uuid.UUID]bool{}
	collect := func(sql string, args ...any) error {
		rows, err := db.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if rows.Scan(&id) == nil {
				seeds[id] = true
			}
		}
		return rows.Err()
	}
	if roles := snap.RolesWith(perm); len(roles) > 0 {
		if err := collect(`SELECT node_id FROM grants WHERE role_id = ANY($1) AND (expires_at IS NULL OR expires_at > now())
			AND ((principal_type = 'user' AND principal_id = $2) OR (principal_type = 'group' AND principal_id = ANY($3)))`,
			roles, userID, groupIDs); err != nil {
			return nil, err
		}
	}
	if err := collect(`SELECT id FROM nodes WHERE owner_id = $1`, userID); err != nil {
		return nil, err
	}
	if nt := snap.Types[typ]; nt.ViaPlacement && nt.AccessPermission == perm {
		for _, g := range groupIDs {
			seeds[g] = true
		}
	}
	if catalog.ServiceOf(perm) == catalog.Platform {
		if u, err := e.ids.GetUser(ctx, userID); err == nil && u.IsAdmin() {
			seeds[catalog.RootID] = true
		}
	}

	ids := make([]uuid.UUID, 0, len(seeds))
	for id := range seeds {
		ids = append(ids, id)
	}
	rows, err = db.Query(ctx, `SELECT path FROM nodes WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	var prefixes []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			prefixes = append(prefixes, p)
		}
	}
	rows.Close()
	slices.Sort(prefixes)
	prefixes = slices.CompactFunc(prefixes, func(a, b string) bool { return strings.HasPrefix(b, a) })

	seen := map[uuid.UUID]bool{}
	var out []tree.Node
	for _, prefix := range prefixes {
		q := `SELECT id, region, tenant_id, parent_id, type, coalesce(external_id, ''), name, owner_id, path, depth, created_at, updated_at
			FROM nodes WHERE type = $1 AND path LIKE $2 || '%'`
		args := []any{typ, prefix}
		if tenantID != nil {
			q += ` AND tenant_id = $3`
			args = append(args, *tenantID)
		}
		rows, err := db.Query(ctx, q+` ORDER BY name LIMIT 5000`, args...)
		if err != nil {
			return nil, err
		}
		var batch []tree.Node
		for rows.Next() {
			var n tree.Node
			if err := rows.Scan(&n.ID, &n.Region, &n.TenantID, &n.ParentID, &n.Type, &n.ExternalID, &n.Name, &n.OwnerID, &n.Path, &n.Depth, &n.CreatedAt, &n.UpdatedAt); err == nil {
				batch = append(batch, n)
			}
		}
		rows.Close()
		for _, n := range batch {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			r, err := e.At(ctx, userID, n, c)
			if err != nil {
				return nil, err
			}
			if r.Has(perm) {
				out = append(out, n)
				if limit > 0 && len(out) >= limit {
					return out, nil
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b tree.Node) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Subject is a person who holds a permission, and why.
type Subject struct {
	UserID uuid.UUID
	Reason Reason
}

// LookupSubjects lists the people who hold perm at node: who can *really*
// reach it, through grants up the chain, group rosters, placement and
// ownership. (A node's grant list is only what was granted there.)
func (e *Engine) LookupSubjects(ctx context.Context, perm string, node tree.Node, c Consistency) ([]Subject, error) {
	db := e.store.Pool
	regions := []string{node.Region, "default"}
	chainIDs := node.Chain()
	candidates := map[uuid.UUID]bool{}
	collect := func(sql string, args ...any) error {
		rows, err := db.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if rows.Scan(&id) == nil {
				candidates[id] = true
			}
		}
		return rows.Err()
	}
	if err := collect(`SELECT owner_id FROM nodes WHERE region = ANY($1) AND id = ANY($2) AND owner_id IS NOT NULL`, regions, chainIDs); err != nil {
		return nil, err
	}
	if err := collect(`SELECT principal_id FROM grants WHERE region = ANY($1) AND node_id = ANY($2) AND principal_type = 'user'`, regions, chainIDs); err != nil {
		return nil, err
	}
	if err := collect(`SELECT m.user_id FROM group_members m WHERE m.group_id IN (
			SELECT principal_id FROM grants WHERE region = ANY($1) AND node_id = ANY($2) AND principal_type = 'group')`, regions, chainIDs); err != nil {
		return nil, err
	}
	// Placement: rosters of the groups above.
	if err := collect(`SELECT m.user_id FROM group_members m JOIN nodes n ON n.region = m.region AND n.id = m.group_id
		WHERE n.id = ANY($1) AND n.type = 'hayat/group'`, chainIDs); err != nil {
		return nil, err
	}
	if catalog.ServiceOf(perm) == catalog.Platform {
		if err := collect(`SELECT id FROM users WHERE system_role = 'admin' AND status = 'active'`); err != nil {
			return nil, err
		}
	}
	var out []Subject
	for id := range candidates {
		ok, why, _, err := e.Check(ctx, id, perm, node, c)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, Subject{UserID: id, Reason: why})
		}
	}
	slices.SortFunc(out, func(a, b Subject) int { return strings.Compare(a.UserID.String(), b.UserID.String()) })
	return out, nil
}

package tree

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/store"
)

// Groups nest the way an org chart does: whoever is on Payment's roster is in
// Engineering too, when Payment is filed under Engineering. So a grant to
// Engineering reaches Payment's people, and Engineering's roster, read whole,
// is its own people plus everyone on a group nested under it.
//
// Belonging goes up; access still only goes down. A grant on Payment reaches
// nothing above it, and placement keeps sibling teams apart (see the authz
// engine): Payment's people don't open what Promo has filed.

// Membership is where one person stands among one tenant's rosters.
type Membership struct {
	// Direct: the rosters they are on, groups and the tenant itself.
	Direct map[uuid.UUID]bool
	// Through: every group they count in only because a group they are on is
	// nested under it, mapped to that group (the one closest under it, for the
	// reasons).
	Through map[uuid.UUID]uuid.UUID
}

// In reports whether they count in group g, directly or through a nested one.
func (m Membership) In(g uuid.UUID) bool {
	_, through := m.Through[g]
	return m.Direct[g] || through
}

// Groups is every group (and the tenant) they count in.
func (m Membership) Groups() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m.Direct)+len(m.Through))
	for g := range m.Direct {
		out = append(out, g)
	}
	for g := range m.Through {
		out = append(out, g)
	}
	return out
}

// GroupAncestors is the groups a group is nested under, outermost first. A
// group's path is /root/<tenant node>/<group>/…/<itself>/, and a group only
// ever sits under the tenant or another group, so these are the segments
// between the tenant and the group itself.
func GroupAncestors(path string) []uuid.UUID {
	var ids []uuid.UUID
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		if id, err := uuid.Parse(seg); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) < 3 {
		return nil
	}
	return ids[1 : len(ids)-1]
}

// LoadMembership reads one person's rosters in a tenant and works out the
// groups they count in through nesting. tenantID is the tenant's id (not its
// node's); with uuid.Nil it reads every tenant they belong to.
func LoadMembership(ctx context.Context, db store.DB, tenantID, userID uuid.UUID) (Membership, error) {
	q := `SELECT n.id, n.type, n.path FROM group_members m JOIN nodes n ON n.region = m.region AND n.id = m.group_id
		WHERE m.user_id = $1`
	args := []any{userID}
	if tenantID != uuid.Nil {
		q += ` AND m.tenant_id = $2`
		args = append(args, tenantID)
	}
	rows, err := db.Query(ctx, q+` ORDER BY n.depth, n.path`, args...)
	if err != nil {
		return Membership{}, err
	}
	defer rows.Close()
	m := Membership{Direct: map[uuid.UUID]bool{}, Through: map[uuid.UUID]uuid.UUID{}}
	type onGroup struct {
		id   uuid.UUID
		path string
	}
	var groups []onGroup
	for rows.Next() {
		var g onGroup
		var typ string
		if err := rows.Scan(&g.id, &typ, &g.path); err != nil {
			return Membership{}, err
		}
		m.Direct[g.id] = true
		if typ == catalog.TypeGroup {
			groups = append(groups, g)
		}
	}
	if err := rows.Err(); err != nil {
		return Membership{}, err
	}
	// Shallowest groups first, so the group recorded for an ancestor is the
	// one closest under it: Engineering is reached "through Payment", not
	// through Payment's own sub-team, when they are on both.
	for _, g := range groups {
		for _, a := range GroupAncestors(g.path) {
			if _, seen := m.Through[a]; !seen && !m.Direct[a] {
				m.Through[a] = g.id
			}
		}
	}
	return m, nil
}

// SubgroupMember is someone who counts on a group's roster only through
// groups nested under it.
type SubgroupMember struct {
	UserID uuid.UUID
	Groups []Node // the nested groups they are on, outermost first
}

// SubgroupMembers lists the people on groups nested under g who aren't on
// g's own roster: they are members of g too, for every grant made to it.
func (t *Service) SubgroupMembers(ctx context.Context, g Node) ([]SubgroupMember, error) {
	rows, err := t.store.Pool.Query(ctx, `SELECT m.user_id, `+prefixed("n")+`
		FROM group_members m JOIN nodes n ON n.region = m.region AND n.id = m.group_id
		WHERE n.region = $1 AND n.tenant_id = $2 AND n.type = 'hayat/group' AND n.path LIKE $3 || '%' AND n.id <> $4
		  AND NOT EXISTS (SELECT 1 FROM group_members d WHERE d.region = $1 AND d.group_id = $4 AND d.user_id = m.user_id)
		ORDER BY n.depth, n.name`, g.Region, g.TenantID, g.Path, g.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubgroupMember
	at := map[uuid.UUID]int{}
	for rows.Next() {
		var u uuid.UUID
		var n Node
		if err := rows.Scan(&u, &n.ID, &n.Region, &n.TenantID, &n.ParentID, &n.Type, &n.ExternalID, &n.Name, &n.OwnerID,
			&n.Path, &n.Depth, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		i, ok := at[u]
		if !ok {
			i = len(out)
			at[u] = i
			out = append(out, SubgroupMember{UserID: u})
		}
		out[i].Groups = append(out[i].Groups, n)
	}
	return out, rows.Err()
}

// prefixed is nodeColumns read through a table alias.
func prefixed(a string) string {
	return fmt.Sprintf(`%[1]s.id, %[1]s.region, %[1]s.tenant_id, %[1]s.parent_id, %[1]s.type, coalesce(%[1]s.external_id, ''), `+
		`%[1]s.name, %[1]s.owner_id, %[1]s.path, %[1]s.depth, %[1]s.created_at, %[1]s.updated_at`, a)
}

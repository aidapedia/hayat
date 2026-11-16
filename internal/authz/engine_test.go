package authz_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/testdb"
	"github.com/aidapedia/hayat/internal/tree"
)

const billing = `
service: billing
version: 1
resource_types:
  - name: ledger
    parents: [hayat/tenant, hayat/group]
    access: { permission: ledger.view, via_placement: true, requires_membership: true }
permissions:
  - { key: ledger.view, scope: ledger }
  - { key: ledger.post, scope: ledger }
default_roles:
  - { slug: accountant, name: Accountant, permissions: [ledger.view, ledger.post] }
`

type world struct {
	t       *testing.T
	ctx     context.Context
	ids     *identity.Service
	tenants *tenancy.Service
	cat     *catalog.Service
	tree    *tree.Service
	engine  *authz.Engine
	users   map[string]uuid.UUID
	tenant  tenancy.Tenant
	tenantN tree.Node
	finance tree.Node
	sub     tree.Node
	l1, l2  tree.Node
	accRole uuid.UUID
}

func build(t *testing.T) *world {
	s := testdb.New(t)
	ctx := context.Background()
	revs := revision.NewReader(s, time.Second)
	w := &world{t: t, ctx: ctx, ids: identity.New(s, "http://auth.test", time.Hour), tenants: tenancy.New(s), users: map[string]uuid.UUID{}}
	w.cat = catalog.New(s, revs)
	w.tree = tree.New(s, w.cat, revs)
	w.engine = authz.New(s, w.cat, w.ids, revs)
	must(t, w.cat.Bootstrap(ctx))

	ada, err := w.ids.CreateAdmin(ctx, "ada@example.com", "Ada", "a long enough password")
	must(t, err)
	w.users["ada"] = ada.ID
	for _, name := range []string{"olive", "ana", "ben", "cy"} {
		u, _, err := w.ids.Invite(ctx, name+"@example.com", name, "test")
		must(t, err)
		_, _ = s.Pool.Exec(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, u.ID)
		w.users[name] = u.ID
	}

	_, err = w.cat.RegisterService(ctx, "billing", "Billing", "", "c_billing", ada.ID, "test")
	must(t, err)
	_, err = w.cat.Publish(ctx, "billing", []byte(billing), "test")
	must(t, err)
	snap, _ := w.cat.Snapshot(ctx, true)
	w.accRole = snap.RolesWith("billing.ledger.post")[0]

	w.tenant, err = w.tenants.Create(ctx, "Acme", "acme", w.users["olive"], "test")
	must(t, err)
	must(t, w.tenants.AddMember(ctx, w.tenant.ID, w.users["ana"], "test"))
	must(t, w.tenants.AddMember(ctx, w.tenant.ID, w.users["ben"], "test"))
	w.tenantN, err = w.tree.ByRef(ctx, catalog.TypeTenant, w.tenant.ID.String())
	must(t, err)

	w.finance, _, err = w.tree.CreateGroup(ctx, w.tenantN.ID, "Finance", "test")
	must(t, err)
	w.sub, _, err = w.tree.CreateGroup(ctx, w.finance.ID, "Payables", "test")
	must(t, err)
	w.l1, _, err = w.tree.UpsertResource(ctx, "billing", tree.ResourceInput{Type: "billing/ledger", ExternalID: "l1", Name: "Ledger 1", ParentID: w.sub.ID})
	must(t, err)
	w.l2, _, err = w.tree.UpsertResource(ctx, "billing", tree.ResourceInput{Type: "billing/ledger", ExternalID: "l2", Name: "Ledger 2",
		ParentType: catalog.TypeTenant, ParentExternalID: w.tenant.ID.String()})
	must(t, err)
	_, err = w.tree.AddGroupMember(ctx, w.finance.ID, w.users["ana"], "test")
	must(t, err)
	return w
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (w *world) can(user, perm string, n tree.Node) bool {
	w.t.Helper()
	n, err := w.tree.Get(w.ctx, n.ID) // re-read: paths change on moves
	must(w.t, err)
	ok, _, _, err := w.engine.Check(w.ctx, w.users[user], perm, n, authz.Consistency{})
	must(w.t, err)
	return ok
}

func TestPlacementCountsGroupsNeverTheTenant(t *testing.T) {
	w := build(t)
	if !w.can("ana", "billing.ledger.view", w.l1) {
		t.Error("Ana is on Finance's roster and Ledger 1 is filed under Finance")
	}
	if w.can("ana", "billing.ledger.post", w.l1) {
		t.Error("placement gives only the access permission")
	}
	if w.can("ana", "billing.ledger.view", w.l2) {
		t.Error("Ledger 2 sits under the tenant; the tenant's roster must not count as placement")
	}
	if w.can("ben", "billing.ledger.view", w.l1) {
		t.Error("Ben is on no group")
	}
	// Moving Ledger 2 under Finance gives Ana access by placement.
	_, _, err := w.tree.Move(w.ctx, w.l2.ID, w.finance.ID, "test")
	must(t, err)
	if !w.can("ana", "billing.ledger.view", w.l2) {
		t.Error("after the move Ledger 2 is filed under Finance")
	}
}

func TestInheritAndEveryoneGrants(t *testing.T) {
	w := build(t)
	g, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.finance.ID, PrincipalType: tree.PrincipalUser,
		PrincipalID: w.users["ben"], RoleID: w.accRole, Inherit: false}, "test")
	must(t, err)
	if w.can("ben", "billing.ledger.post", w.l1) {
		t.Error("a non-inheriting grant on Finance must not reach Ledger 1 below it")
	}
	_, err = w.tree.RemoveGrant(w.ctx, g.ID, "test")
	must(t, err)
	_, _, err = w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.finance.ID, PrincipalType: tree.PrincipalUser,
		PrincipalID: w.users["ben"], RoleID: w.accRole, Inherit: true}, "test")
	must(t, err)
	if !w.can("ben", "billing.ledger.post", w.l1) {
		t.Error("an inheriting grant on Finance reaches Ledger 1")
	}

	// The tenant as principal means everyone on its roster.
	_, _, err = w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.l2.ID, PrincipalType: tree.PrincipalGroup,
		PrincipalID: w.tenantN.ID, RoleID: w.accRole}, "test")
	must(t, err)
	if !w.can("ana", "billing.ledger.post", w.l2) || w.can("cy", "billing.ledger.post", w.l2) {
		t.Error("a grant to the tenant reaches members and only members")
	}

	got, err := w.engine.LookupResources(w.ctx, w.users["ben"], "billing.ledger.post", "billing/ledger", nil, authz.Consistency{}, 0)
	must(t, err)
	names := []string{}
	for _, n := range got {
		names = append(names, n.Name)
	}
	if !slices.Equal(names, []string{"Ledger 1", "Ledger 2"}) {
		t.Errorf("LookupResources: %v", names)
	}

	subjects, err := w.engine.LookupSubjects(w.ctx, "billing.ledger.view", w.l1, authz.Consistency{})
	must(t, err)
	var who []uuid.UUID
	for _, s := range subjects {
		who = append(who, s.UserID)
	}
	for _, name := range []string{"ana", "ben", "olive"} {
		if !slices.Contains(who, w.users[name]) {
			t.Errorf("LookupSubjects is missing %s", name)
		}
	}
	if slices.Contains(who, w.users["ada"]) || slices.Contains(who, w.users["cy"]) {
		t.Error("LookupSubjects included someone without access")
	}
}

func TestOwnersAdminsAndMembership(t *testing.T) {
	w := build(t)
	if !w.can("olive", "billing.ledger.post", w.l2) || !w.can("olive", catalog.PermResourcesGrant, w.finance) {
		t.Error("the tenant's owner can do everything in it")
	}
	if !w.can("ada", catalog.PermResourcesOrganise, w.finance) {
		t.Error("an instance admin holds platform permissions")
	}
	if w.can("ada", "billing.ledger.view", w.l1) {
		t.Error("an instance admin must not read a service's data")
	}
	if _, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.l2.ID, PrincipalType: tree.PrincipalUser,
		PrincipalID: w.users["cy"], RoleID: w.accRole}, "test"); !errors.Is(err, tree.ErrNotMember) {
		t.Errorf("granting to a non-member: %v", err)
	}
	if _, _, err := w.tree.AddGrant(w.ctx, tree.GrantInput{NodeID: w.l1.ID, PrincipalType: tree.PrincipalUser,
		PrincipalID: w.users["ben"], RoleID: catalog.RoleTenantAdmin}, "test"); err == nil {
		t.Error("tenant admin can't be granted on a ledger")
	}

	// Leaving the tenant takes the rosters and grants with it.
	must(t, w.tenants.RemoveMember(w.ctx, w.tenant.ID, w.users["ana"], "test"))
	if w.can("ana", "billing.ledger.view", w.l1) {
		t.Error("Ana left the tenant but kept access")
	}

	if _, err := w.tree.DeleteGroup(w.ctx, w.finance.ID, "test"); !errors.Is(err, tree.ErrNotEmpty) {
		t.Errorf("deleting a group with things in it: %v", err)
	}
	if _, _, err := w.tree.Move(w.ctx, w.finance.ID, w.sub.ID, "test"); !errors.Is(err, tree.ErrCycle) {
		t.Errorf("moving a group under its own child: %v", err)
	}
}

func TestExplanation(t *testing.T) {
	w := build(t)
	n, _ := w.tree.Get(w.ctx, w.l1.ID)
	ok, why, _, err := w.engine.Check(w.ctx, w.users["ana"], "billing.ledger.view", n, authz.Consistency{})
	must(t, err)
	if !ok || why.Kind != authz.ReasonPlacement || why.NodeID != w.finance.ID {
		t.Fatalf("want placement through Finance, got %+v", why)
	}
}

// Groups nest like an org chart: Ben is on Payables, so he is in Finance too.
// Belonging goes up, access still only goes down, and sibling teams stay apart.
//
//	Acme
//	├─ Ledger 2
//	└─ Finance            (Ana)
//	   ├─ Ledger 4
//	   ├─ Payables        (Ben)
//	   │  ├─ Ledger 1
//	   │  └─ AP Team      (Ben)
//	   └─ Receivables
//	      └─ Ledger 3
func TestNestedGroupsBelongUp(t *testing.T) {
	w := build(t)
	ctx := w.ctx
	ap, _, err := w.tree.CreateGroup(ctx, w.sub.ID, "AP Team", "test")
	must(t, err)
	recv, _, err := w.tree.CreateGroup(ctx, w.finance.ID, "Receivables", "test")
	must(t, err)
	l3, _, err := w.tree.UpsertResource(ctx, "billing", tree.ResourceInput{Type: "billing/ledger", ExternalID: "l3", Name: "Ledger 3", ParentID: recv.ID})
	must(t, err)
	l4, _, err := w.tree.UpsertResource(ctx, "billing", tree.ResourceInput{Type: "billing/ledger", ExternalID: "l4", Name: "Ledger 4", ParentID: w.finance.ID})
	must(t, err)
	for _, g := range []uuid.UUID{w.sub.ID, ap.ID} {
		_, err = w.tree.AddGroupMember(ctx, g, w.users["ben"], "test")
		must(t, err)
	}

	// Placement: his team's things and what is filed above it, never a sibling's.
	if !w.can("ben", "billing.ledger.view", w.l1) {
		t.Error("Ledger 1 is filed with Payables, Ben's team")
	}
	if !w.can("ben", "billing.ledger.view", l4) {
		t.Error("Ledger 4 is filed under Finance, and Ben is in Finance through Payables")
	}
	if w.can("ben", "billing.ledger.view", l3) {
		t.Error("Ledger 3 belongs to Receivables, a sibling team: being in Finance must not open it")
	}
	if !w.can("ana", "billing.ledger.view", l3) {
		t.Error("Ana is on Finance's own roster, so everything filed under Finance opens to her")
	}
	n4, _ := w.tree.Get(ctx, l4.ID)
	_, why, _, err := w.engine.Check(ctx, w.users["ben"], "billing.ledger.view", n4, authz.Consistency{})
	must(t, err)
	if why.Kind != authz.ReasonPlacement || why.NodeID != w.finance.ID || why.ThroughID != w.sub.ID {
		t.Errorf("want placement in Finance through Payables, got %+v", why)
	}

	// A grant to Finance reaches Payables' people; one to Payables reaches nobody above.
	_, _, err = w.tree.AddGrant(ctx, tree.GrantInput{NodeID: w.l2.ID, PrincipalType: tree.PrincipalGroup,
		PrincipalID: w.finance.ID, RoleID: w.accRole}, "test")
	must(t, err)
	_, _, err = w.tree.AddGrant(ctx, tree.GrantInput{NodeID: l4.ID, PrincipalType: tree.PrincipalGroup,
		PrincipalID: w.sub.ID, RoleID: w.accRole}, "test")
	must(t, err)
	if !w.can("ben", "billing.ledger.post", w.l2) {
		t.Error("a grant to Finance reaches Ben, who is on Payables under it")
	}
	if w.can("ana", "billing.ledger.post", l4) {
		t.Error("a grant to Payables must not reach Ana, who is only on Finance above it")
	}
	n2, _ := w.tree.Get(ctx, w.l2.ID)
	_, why, _, err = w.engine.Check(ctx, w.users["ben"], "billing.ledger.post", n2, authz.Consistency{})
	must(t, err)
	if why.PrincipalID != w.finance.ID || why.ThroughID != w.sub.ID || !strings.Contains(why.String(), `"Payables"`) {
		t.Errorf("want the Finance grant, through Payables (the group closest under Finance, not AP Team): %+v / %s", why, why)
	}

	// The lookups agree with Check.
	got, err := w.engine.LookupResources(ctx, w.users["ben"], "billing.ledger.view", "billing/ledger", nil, authz.Consistency{}, 0)
	must(t, err)
	var names []string
	for _, n := range got {
		names = append(names, n.Name)
	}
	if !slices.Equal(names, []string{"Ledger 1", "Ledger 2", "Ledger 4"}) {
		t.Errorf("LookupResources for Ben: %v", names)
	}
	holds := func(n tree.Node, name string) bool {
		n, _ = w.tree.Get(ctx, n.ID)
		subjects, err := w.engine.LookupSubjects(ctx, "billing.ledger.view", n, authz.Consistency{})
		must(t, err)
		return slices.ContainsFunc(subjects, func(s authz.Subject) bool { return s.UserID == w.users[name] })
	}
	if !holds(l4, "ben") || !holds(w.l2, "ben") || holds(l3, "ben") {
		t.Error("LookupSubjects disagrees with Check about Ben")
	}

	// Who reaches a node, and the tenant's grants, say "through Payables".
	access, err := w.tree.NodeAccess(ctx, n4)
	must(t, err)
	if !slices.ContainsFunc(access, func(a tree.Access) bool {
		return a.UserID == w.users["ben"] && a.Via == tree.ViaPlacement && a.GroupID == w.finance.ID && a.ThroughName == "Payables"
	}) {
		t.Errorf("NodeAccess on Ledger 4 should show Ben in Finance through Payables: %+v", access)
	}
	access, err = w.tree.NodeAccess(ctx, n2)
	must(t, err)
	if !slices.ContainsFunc(access, func(a tree.Access) bool {
		return a.UserID == w.users["ben"] && a.Via == tree.ViaGroup && a.GroupID == w.finance.ID && a.ThroughID == w.sub.ID
	}) {
		t.Errorf("NodeAccess on Ledger 2 should show the Finance grant reaching Ben: %+v", access)
	}
	grants, err := w.tree.MemberGrants(ctx, w.tenant.ID, []uuid.UUID{w.users["ben"]}, nil)
	must(t, err)
	financeGrants := 0
	for _, g := range grants {
		if g.Via == tree.ViaGroup && g.GroupID == w.finance.ID {
			financeGrants++
		}
	}
	if financeGrants != 1 {
		t.Errorf("MemberGrants should list the Finance grant once for Ben (he is on two groups under it), got %d: %+v", financeGrants, grants)
	}

	// Finance's roster, read whole: Ana on it, Ben through Payables and AP Team.
	nested, err := w.tree.SubgroupMembers(ctx, w.finance)
	must(t, err)
	if len(nested) != 1 || nested[0].UserID != w.users["ben"] || len(nested[0].Groups) != 2 ||
		nested[0].Groups[0].ID != w.sub.ID || nested[0].Groups[1].ID != ap.ID {
		t.Errorf("SubgroupMembers of Finance: %+v", nested)
	}

	// Moving Payables out of Finance takes Ben out of Finance.
	_, _, err = w.tree.Move(ctx, w.sub.ID, w.tenantN.ID, "test")
	must(t, err)
	if w.can("ben", "billing.ledger.post", w.l2) {
		t.Error("after the move Payables is no longer under Finance, so the Finance grant no longer reaches Ben")
	}
	n4, _ = w.tree.Get(ctx, l4.ID)
	_, why, _, err = w.engine.Check(ctx, w.users["ben"], "billing.ledger.view", n4, authz.Consistency{})
	must(t, err)
	if why.Kind != authz.ReasonGrant || why.PrincipalID != w.sub.ID {
		t.Errorf("Ben keeps Ledger 4 only through the grant to Payables, not by placement in Finance: %+v", why)
	}
}

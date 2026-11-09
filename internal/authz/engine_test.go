package authz_test

import (
	"context"
	"errors"
	"slices"
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
	_, err = w.cat.Approve(ctx, "billing", 1, []string{"billing"}, ada.ID)
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

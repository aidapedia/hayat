package catalog_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/testdb"
)

const billingV1 = `
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
  - { slug: accountant, name: Accountant, permissions: [ledger.view, ledger.post], applies_to: [ledger, hayat/group] }
`

func setup(t *testing.T) (*catalog.Service, context.Context, *identity.Service) {
	s := testdb.New(t)
	ctx := context.Background()
	c := catalog.New(s, revision.NewReader(s, time.Second))
	if err := c.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	return c, ctx, identity.New(s, "http://auth.test", time.Hour)
}

func TestPlatformCatalog(t *testing.T) {
	c, ctx, _ := setup(t)
	snap, err := c.Snapshot(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Known(catalog.PermResourcesGrant) || !snap.CanParent(catalog.TypeTenant, catalog.TypeGroup) {
		t.Fatal("platform catalog missing")
	}
	if !snap.CanContain(catalog.TypeTenant, catalog.TypeGroup) || snap.CanContain(catalog.TypeGroup, catalog.TypeTenant) {
		t.Fatal("type containment wrong")
	}
	if len(snap.Roles[catalog.RoleTenantAdmin].Permissions) != 5 {
		t.Fatalf("tenant admin role: %+v", snap.Roles[catalog.RoleTenantAdmin])
	}
}

func TestContractLifecycle(t *testing.T) {
	c, ctx, ids := setup(t)
	owner, _ := ids.CreateAdmin(ctx, "owner@example.com", "Owner", "a long enough password")
	if _, err := c.RegisterService(ctx, "billing", "Billing", "", "c_billing", owner.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RegisterService(ctx, "payments", "Payments", "", "c_payments", owner.ID, "test"); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Publish(ctx, "billing", []byte(strings.Replace(billingV1, "scope: ledger }", "scope: nowhere }", 1)), "test"); err == nil {
		t.Fatal("unknown scope accepted")
	}
	v1, err := c.Publish(ctx, "billing", []byte(billingV1), "test")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Status != "pending" || !slices.Equal(v1.Namespaces, []string{"billing"}) {
		t.Fatalf("v1: %+v", v1)
	}
	if _, err := c.Publish(ctx, "billing", []byte(strings.Replace(billingV1, "version: 1", "version: 2", 1)), "test"); !errors.Is(err, catalog.ErrPendingExists) {
		t.Fatalf("second pending version: %v", err)
	}
	v1, err = c.Approve(ctx, "billing", 1, []string{"billing"}, owner.ID)
	if err != nil || v1.Status != "active" {
		t.Fatalf("approve: %v %+v", err, v1)
	}
	snap, _ := c.Snapshot(ctx, true)
	if lt := snap.Types["billing/ledger"]; lt.AccessPermission != "billing.ledger.view" || !lt.ViaPlacement {
		t.Fatalf("ledger type: %+v", lt)
	}
	if len(snap.RolesWith("billing.ledger.post")) != 1 {
		t.Fatal("contract role missing")
	}

	// Removing a permission is refused: contracts only add.
	v2 := strings.Replace(strings.Replace(billingV1, "version: 1", "version: 2", 1), "  - { key: ledger.post, scope: ledger }\n", "", 1)
	v2 = strings.Replace(v2, "[ledger.view, ledger.post]", "[ledger.view]", 1)
	if _, err := c.Publish(ctx, "billing", []byte(v2), "test"); err == nil || !strings.Contains(err.Error(), "can only add") {
		t.Fatalf("removal accepted: %v", err)
	}

	// Payments files a type under billing's ledger: billing must approve too.
	payments := `
service: payments
version: 1
resource_types:
  - { name: payout, parents: [billing/ledger] }
permissions:
  - { key: payout.send, scope: payout }
`
	pv, err := c.Publish(ctx, "payments", []byte(payments), "test")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pv.Namespaces, []string{"payments", "billing"}) {
		t.Fatalf("namespaces: %v", pv.Namespaces)
	}
	pv, _ = c.Approve(ctx, "payments", 1, []string{"payments"}, owner.ID)
	if pv.Status != "pending" {
		t.Fatal("activated without billing's approval")
	}
	pv, _ = c.Approve(ctx, "payments", 1, []string{"billing"}, owner.ID)
	if pv.Status != "active" {
		t.Fatalf("both namespaces approved, still %s", pv.Status)
	}
	snap, _ = c.Snapshot(ctx, true)
	if !snap.CanContain(catalog.TypeTenant, "payments/payout") {
		t.Fatal("payout should be reachable from a tenant through the ledger")
	}
}

func TestRoles(t *testing.T) {
	c, ctx, _ := setup(t)
	r, err := c.CreateRole(ctx, catalog.RoleInput{Name: "Organiser", Permissions: []string{catalog.PermResourcesOrganise}}, "test")
	if err != nil || r.Slug != "organiser" {
		t.Fatalf("create: %v %+v", err, r)
	}
	if _, err := c.CreateRole(ctx, catalog.RoleInput{Name: "Bad", Permissions: []string{"nope.nothing"}}, "test"); err == nil {
		t.Fatal("unknown permission accepted")
	}
	if err := c.DeleteRole(ctx, catalog.RoleTenantAdmin, "test"); !errors.Is(err, catalog.ErrRoleBuiltin) {
		t.Fatalf("deleting a built-in: %v", err)
	}
	if err := c.DeleteRole(ctx, r.ID, "test"); err != nil {
		t.Fatal(err)
	}
}

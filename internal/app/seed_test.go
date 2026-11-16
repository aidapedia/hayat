package app_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/app"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/testdb"
	"github.com/aidapedia/hayat/internal/tree"
)

func newSeedApp(t *testing.T, dev bool) *app.App {
	t.Helper()
	cfg := config.Config{Region: "default", Issuer: "http://localhost:8443", ConsoleURL: consoleURL, MasterKey: make([]byte, 32),
		SessionTTL: time.Hour, AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Dev: dev}
	a, err := app.New(cfg, testdb.New(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSeed(t *testing.T) {
	ctx := context.Background()
	a := newSeedApp(t, true)

	created, err := a.Seed(ctx)
	if err != nil || !created {
		t.Fatalf("first seed: created=%v err=%v", created, err)
	}
	if created, err := a.Seed(ctx); err != nil || created {
		t.Fatalf("second seed should do nothing: created=%v err=%v", created, err)
	}

	for _, acc := range app.SeedAccounts {
		if _, err := a.IDs.Authenticate(ctx, acc.Email, app.SeedPassword); err != nil {
			t.Fatalf("%s can't sign in: %v", acc.Email, err)
		}
	}
	admin, _ := a.IDs.GetUserByEmail(ctx, "admin@hayat.test")
	if !admin.IsAdmin() {
		t.Fatal("seed admin should be an instance admin")
	}
	bob, _ := a.IDs.GetUserByEmail(ctx, "bob@hayat.test")
	if bob.IsAdmin() {
		t.Fatal("bob should not be an instance admin")
	}
	tenants, err := a.Tenants.ForUser(ctx, bob.ID)
	if err != nil || len(tenants) != 1 || tenants[0].Name != "Acme" {
		t.Fatalf("bob's tenants = %+v, err %v; want Acme", tenants, err)
	}
	if n, _ := a.Tenants.Count(ctx); n != 2 {
		t.Fatalf("tenants = %d, want 2", n)
	}
	seedOrgChart(t, a, tenants[0].ID)
}

// The seeded Acme reads like a company: teams nest under Engineering, each
// lead runs their own team only, and the head of engineering runs them all.
func seedOrgChart(t *testing.T, a *app.App, acmeID uuid.UUID) {
	ctx := context.Background()
	nodes, err := a.Tree.TenantNodes(ctx, acmeID)
	if err != nil {
		t.Fatal(err)
	}
	group := map[string]tree.Node{}
	for _, n := range nodes {
		group[n.Name] = n
	}
	user := func(name string) uuid.UUID {
		u, err := a.IDs.GetUserByEmail(ctx, name+"@hayat.test")
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}

	nested, err := a.Tree.SubgroupMembers(ctx, group["Engineering"])
	if err != nil {
		t.Fatal(err)
	}
	in := map[uuid.UUID]bool{}
	for _, m := range nested {
		in[m.UserID] = true
	}
	for _, name := range []string{"paula", "pete", "priya", "leo", "mia", "carol"} {
		if !in[user(name)] {
			t.Errorf("%s is on a team under Engineering, so is in Engineering", name)
		}
	}
	if in[user("mona")] || in[user("bob")] {
		t.Error("Mona is in Marketing, and Bob is on Engineering's own roster: neither comes through a team")
	}

	can := func(name, perm, at string) bool {
		ok, _, _, err := a.Engine.Check(ctx, user(name), perm, group[at], authz.Consistency{})
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !can("paula", catalog.PermGroupsManage, "Payment") || can("paula", catalog.PermGroupsManage, "Promo") {
		t.Error("Paula leads Payment and only Payment")
	}
	if can("paula", catalog.PermResourcesOrganise, "Payment") {
		t.Error("a team lead doesn't reshape the tree")
	}
	if can("pete", catalog.PermGroupsManage, "Payment") {
		t.Error("Pete is a member of Payment, not its lead")
	}
	if !can("bob", catalog.PermGroupsManage, "Promo") || !can("bob", catalog.PermResourcesOrganise, "Payment") {
		t.Error("Bob manages Engineering and every team under it")
	}
	if can("bob", catalog.PermGroupsManage, "Marketing") {
		t.Error("Bob's grant is on Engineering; Marketing isn't under it")
	}
}

func TestSeedRefusedOutsideDev(t *testing.T) {
	if _, err := newSeedApp(t, false).Seed(context.Background()); err == nil {
		t.Fatal("seed must refuse without HAYAT_DEV")
	}
}

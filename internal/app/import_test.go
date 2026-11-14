package app_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/internal/app"
	"github.com/aidapedia/hayat/internal/authz"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/sdk/go/hayat"
)

// The shape of tabletsgo's contract, trimmed to what the test asks about.
const consoleContract = `
service: console
version: 1
resource_types:
  - name: connection
    parents: [hayat/tenant, hayat/group]
    access: { permission: connection.open, via_placement: true, requires_membership: true }
permissions:
  - { key: instance.manage, scope: hayat/root }
  - { key: storage.manage, scope: hayat/tenant }
  - { key: connections.manage, scope: connection }
  - { key: connection.open, scope: connection }
default_roles:
  - { slug: admin, name: Console admin, permissions: [instance.manage], applies_to: [hayat/root] }
  - { slug: owner, name: Owner, permissions: [storage.manage, connections.manage, connection.open], applies_to: [hayat/root, hayat/tenant] }
  - { slug: connection-user, name: Connection user, permissions: [connection.open], applies_to: [hayat/tenant, hayat/group, console/connection] }
`

func legacyHash(password string) string {
	return fmt.Sprintf("$sha256$%x", sha256.Sum256([]byte(password)))
}

var (
	ownerID  = "11111111-1111-4111-8111-111111111111"
	memberID = "22222222-2222-4222-8222-222222222222"
	otherID  = "33333333-3333-4333-8333-333333333333"
	wsID     = "44444444-4444-4444-8444-444444444444"
	pendID   = "55555555-5555-4555-8555-555555555555"
)

func consoleBundle() app.Bundle {
	return app.Bundle{
		Format: app.BundleFormat, Version: 1, Source: "console",
		Service: app.BundleService{ID: "console", DisplayName: "Console", Contract: consoleContract},
		Users: []app.BundleUser{
			{ID: ownerID, Email: "owner@example.com", Name: "Olive", PasswordHash: legacyHash("olive-password"), Status: "active", Zoneinfo: "Asia/Jakarta", Admin: true,
				Appearance: &identity.Appearance{Preset: "slate-rail", Mode: "light", Accent: "#2563EB", SidebarBackground: "#0f172a"}},
			{ID: memberID, Email: "member@example.com", Name: "Mo", PasswordHash: legacyHash("mo-password!!"), Status: "active"},
			{ID: otherID, Email: "other@example.com", Name: "Oz", PasswordHash: legacyHash("oz-password!!"), Status: "active"},
			{ID: pendID, Email: "pending@example.com", Name: "Pat", Status: "pending"},
		},
		RootGrants: []app.BundleRootGrant{{UserID: ownerID, Role: "console.admin"}},
		Tenants: []app.BundleTenant{{
			ID: wsID, Name: "Acme", OwnerID: ownerID, Members: []string{ownerID, memberID, otherID, pendID},
			Groups: []app.BundleGroup{{Key: "g1", Name: "Promotions", Members: []string{memberID}}},
			Resources: []app.BundleResource{
				{Type: "console/connection", ExternalID: "conn-promo", Name: "promo-db", Parent: "g1", OwnerID: ownerID},
				{Type: "console/connection", ExternalID: "conn-orders", Name: "orders-db", OwnerID: ownerID},
			},
			Grants: []app.BundleGrant{
				{On: "tenant", Principal: "user:" + ownerID, Role: "hayat.tenant-admin", Inherit: true},
				{On: "tenant", Principal: "user:" + ownerID, Role: "console.owner", Inherit: true},
				{On: "resource:console/connection:conn-orders", Principal: "user:" + otherID, Role: "console.connection-user", Inherit: true},
				{On: "resource:console/dashboard:d1", Principal: "user:" + otherID, Role: "console.connection-user"},
			},
		}},
	}
}

func check(t *testing.T, a *app.App, user, perm, typ, ext string) bool {
	t.Helper()
	n, err := a.Tree.ByRef(context.Background(), typ, ext)
	must(t, err)
	ok, _, _, err := a.Engine.Check(context.Background(), uuid.MustParse(user), perm, n, authz.Consistency{Fresh: true})
	must(t, err)
	return ok
}

func TestImport(t *testing.T) {
	ctx := context.Background()
	a := newSeedApp(t, false)
	res, err := a.Import(ctx, consoleBundle(), app.ImportOptions{AppURL: "https://console.example.com"})
	must(t, err)

	if res.UsersCreated != 4 || res.TenantsCreated != 1 || res.Groups != 1 || res.Resources != 2 {
		t.Fatalf("counts: %+v", res)
	}
	if !res.ServiceRegistered || res.ClientID == "" || res.ClientSecret == "" || res.WebClientID == "" || res.ContractVersion != 1 {
		t.Fatalf("service setup: %+v", res)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("only the grant on an unmirrored dashboard should be skipped: %v", res.Warnings)
	}

	// Ids are kept, the tabletsgo admin became Hayat's first admin, the zone came along.
	owner, err := a.IDs.GetUser(ctx, uuid.MustParse(ownerID))
	must(t, err)
	if !owner.IsAdmin() || owner.Zoneinfo != "Asia/Jakarta" || !owner.EmailVerified {
		t.Fatalf("owner: %+v", owner)
	}
	// So did the theme they chose there; nobody else had one.
	if look, err := a.IDs.Appearance(ctx, uuid.MustParse(ownerID)); err != nil || look == nil || look.Accent != "#2563eb" || look.SidebarBackground != "#0f172a" {
		t.Fatalf("owner's appearance: %+v %v", look, err)
	}
	if look, _ := a.IDs.Appearance(ctx, uuid.MustParse(memberID)); look != nil {
		t.Fatalf("an account imported without a theme has none: %+v", look)
	}
	if pat, _ := a.IDs.GetUser(ctx, uuid.MustParse(pendID)); pat.Status != "pending" || pat.EmailVerified || pat.HasPassword {
		t.Fatalf("a pending account stays pending and unverified: %+v", pat)
	}
	if tn, err := a.Tenants.Get(ctx, uuid.MustParse(wsID)); err != nil || tn.OwnerID.String() != ownerID || tn.MemberCount != 4 {
		t.Fatalf("tenant: %+v %v", tn, err)
	}

	// The access they had carries over through Hayat's rules.
	if !check(t, a, memberID, "console.connection.open", "console/connection", "conn-promo") {
		t.Fatal("Mo is on Promotions' roster, where promo-db is filed")
	}
	if check(t, a, memberID, "console.connection.open", "console/connection", "conn-orders") {
		t.Fatal("Mo was never given orders-db")
	}
	if !check(t, a, otherID, "console.connection.open", "console/connection", "conn-orders") {
		t.Fatal("Oz was on orders-db's access list")
	}
	if !check(t, a, ownerID, "console.connections.manage", "console/connection", "conn-promo") {
		t.Fatal("the owner manages every connection")
	}
	root, err := a.Tree.Get(ctx, catalog.RootID)
	must(t, err)
	if ok, _, _, _ := a.Engine.Check(ctx, uuid.MustParse(ownerID), "console.instance.manage", root, authz.Consistency{Fresh: true}); !ok {
		t.Fatal("the root grant makes the owner the console's admin")
	}

	// The old password signs in, and is upgraded to argon2id on the way.
	if _, err := a.IDs.Authenticate(ctx, "member@example.com", "mo-password!!"); err != nil {
		t.Fatalf("legacy password: %v", err)
	}
	var hash string
	must(t, a.Store.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, memberID).Scan(&hash))
	if hash[:10] != "$argon2id$" {
		t.Fatalf("the legacy hash wasn't replaced: %s", hash)
	}
	if _, err := a.IDs.Authenticate(ctx, "member@example.com", "mo-password!!"); err != nil {
		t.Fatalf("after the upgrade: %v", err)
	}
	if _, err := a.IDs.Authenticate(ctx, "member@example.com", "wrong-password"); err == nil {
		t.Fatal("a wrong password must fail")
	}

	// Running it again changes nothing.
	again, err := a.Import(ctx, consoleBundle(), app.ImportOptions{})
	must(t, err)
	if again.UsersCreated != 0 || again.UsersMatched != 4 || again.TenantsSkipped != 1 || again.ServiceRegistered || len(again.IDMap) != 0 {
		t.Fatalf("re-run: %+v", again)
	}
}

// An address Hayat already knows keeps its Hayat id; the result maps the
// bundle's id onto it so the exporting system can rewrite its references.
func TestImportMatchesExistingAddress(t *testing.T) {
	ctx := context.Background()
	a := newSeedApp(t, false)
	existing, err := a.IDs.CreateActive(ctx, "member@example.com", "Mo (Hayat)", "a long enough password", true, "test")
	must(t, err)
	res, err := a.Import(ctx, consoleBundle(), app.ImportOptions{})
	must(t, err)
	if res.IDMap[memberID] != existing.ID.String() || res.UsersMatched != 1 {
		t.Fatalf("id map: %+v", res)
	}
	if owner, _ := a.IDs.GetUser(ctx, uuid.MustParse(ownerID)); owner.IsAdmin() {
		t.Fatal("Hayat already had an admin, so the bundle's admin isn't made one")
	}
	if !check(t, a, existing.ID.String(), "console.connection.open", "console/connection", "conn-promo") {
		t.Fatal("the matched account carries the access the bundle gave its old id")
	}
}

// The directory calls a service uses for its tenant switcher and member lists.
func TestDirectoryTenants(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	res, err := e.a.Import(ctx, consoleBundle(), app.ImportOptions{})
	must(t, err)
	hz, err := hayat.New(ctx, hayat.Config{APIURL: e.api, Issuer: e.auth, ClientID: res.ClientID, ClientSecret: res.ClientSecret})
	must(t, err)

	tenants, err := hz.Directory.ListUserTenants(ctx, connect.NewRequest(&authzv1.ListUserTenantsRequest{UserId: memberID}))
	must(t, err)
	if len(tenants.Msg.Tenants) != 1 || tenants.Msg.Tenants[0].Id != wsID || tenants.Msg.Tenants[0].Name != "Acme" ||
		tenants.Msg.Tenants[0].NodeId == "" || tenants.Msg.Tenants[0].OwnerUserId != ownerID {
		t.Fatalf("tenants: %+v", tenants.Msg.Tenants)
	}
	members, err := hz.Directory.ListTenantMembers(ctx, connect.NewRequest(&authzv1.ListTenantMembersRequest{TenantId: wsID}))
	must(t, err)
	if len(members.Msg.Members) != 4 {
		t.Fatalf("members: %+v", members.Msg.Members)
	}
	owners := 0
	for _, m := range members.Msg.Members {
		if m.IsOwner {
			owners++
			if m.User.Id != ownerID || m.User.Email != "owner@example.com" {
				t.Fatalf("owner: %+v", m.User)
			}
		}
	}
	if owners != 1 {
		t.Fatalf("one owner, got %d", owners)
	}
	if _, err := hz.Directory.ListTenantMembers(ctx, connect.NewRequest(&authzv1.ListTenantMembersRequest{TenantId: uuid.NewString()})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("an unknown tenant is NotFound, got %v", err)
	}
}

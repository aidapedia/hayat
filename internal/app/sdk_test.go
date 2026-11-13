package app_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/authz/v1/authzv1connect"
	"github.com/aidapedia/hayat/internal/apiserver"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/tree"
	"github.com/aidapedia/hayat/sdk/go/hayat"
)

const billingContract = `
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

// TestServiceJourney is a service's whole life with Hayat, through the real
// HTTP servers: an admin registers it, it publishes its contract from "CI",
// an owner approves, it mirrors a resource, and its checks follow policy
// changes as they happen.
func TestServiceJourney(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	admin, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password")
	must(t, err)
	token, _ := e.signIn("root@example.com", "a long enough password")
	registry := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(token))

	// 1. An instance admin registers the service and gets its API credentials.
	reg, err := registry.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", DisplayName: "Billing", OwnerEmail: "root@example.com"}))
	must(t, err)
	hz, err := hayat.New(ctx, hayat.Config{APIURL: e.api, Issuer: e.auth, ClientID: reg.Msg.ClientId, ClientSecret: reg.Msg.ClientSecret})
	must(t, err)
	go hz.Watch(ctx)

	// 2. The service publishes its contract; its owner approves it.
	pub, err := hz.Schema.PublishContract(ctx, connect.NewRequest(&authzv1.PublishContractRequest{Manifest: billingContract}))
	must(t, err)
	if pub.Msg.Status != "pending" {
		t.Fatalf("a new contract waits for approval, got %s", pub.Msg.Status)
	}
	approved, err := registry.ApproveContract(ctx, connect.NewRequest(&adminv1.ApproveContractRequest{ServiceId: "billing", Version: 1}))
	must(t, err)
	if approved.Msg.Contract.Status != "active" {
		t.Fatalf("approved contract: %s", approved.Msg.Contract.Status)
	}

	// 3. A tenant with a Finance group; Ana is on its roster, Ben is a member only.
	ana, _, _ := e.a.IDs.Invite(ctx, "ana@example.com", "Ana", "test")
	ben, _, _ := e.a.IDs.Invite(ctx, "ben@example.com", "Ben", "test")
	for _, id := range []string{ana.ID.String(), ben.ID.String()} {
		_, _ = e.a.Store.Pool.Exec(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, id)
	}
	tn, err := e.a.Tenants.Create(ctx, "Acme", "acme", admin.ID, "test")
	must(t, err)
	must(t, e.a.Tenants.AddMember(ctx, tn.ID, ana.ID, "test"))
	must(t, e.a.Tenants.AddMember(ctx, tn.ID, ben.ID, "test"))
	tenantNode, _ := e.a.Tree.ByRef(ctx, catalog.TypeTenant, tn.ID.String())
	finance, _, err := e.a.Tree.CreateGroup(ctx, tenantNode.ID, "Finance", "test")
	must(t, err)
	_, err = e.a.Tree.AddGroupMember(ctx, finance.ID, ana.ID, "test")
	must(t, err)

	// 4. The service files a ledger under Finance and checks at once
	//    (read-your-writes), then lists what Ana can open.
	ledger := hayat.Ref("billing/ledger", "l-1")
	_, err = hz.UpsertResource(ctx, ledger, "Ledger 1", hayat.Resource{NodeID: finance.ID.String()}, "")
	must(t, err)
	if ok, err := hz.Check(ctx, ana.ID.String(), "billing.ledger.view", ledger); err != nil || !ok {
		t.Fatalf("Ana is on Finance's roster: %v %v", ok, err)
	}
	if ok, _ := hz.Check(ctx, ben.ID.String(), "billing.ledger.view", ledger); ok {
		t.Fatal("Ben is on no group")
	}
	ids, err := hz.Lookup(ctx, ana.ID.String(), "billing.ledger.view", "billing/ledger", "")
	must(t, err)
	if len(ids) != 1 || ids[0] != "l-1" {
		t.Fatalf("Lookup: %v", ids)
	}
	if ok, _ := hz.Check(ctx, ana.ID.String(), "billing.ledger.view", hayat.Ref("billing/ledger", "nope")); ok {
		t.Fatal("an unknown resource must be a no")
	}

	// 5. The "no" for Ben is cached now. A grant made elsewhere reaches the
	//    service through Watch, well before the 10-second cache would expire.
	snap, _ := e.a.Catalog.Snapshot(ctx, true)
	accountant := snap.RolesWith("billing.ledger.post")[0]
	ledgerNode, _ := e.a.Tree.ByRef(ctx, "billing/ledger", "l-1")
	_, _, err = e.a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: ledgerNode.ID, PrincipalType: tree.PrincipalUser, PrincipalID: ben.ID, RoleID: accountant}, "test")
	must(t, err)
	deadline := time.Now().Add(4 * time.Second)
	for {
		ok, err := hz.Check(ctx, ben.ID.String(), "billing.ledger.post", ledger)
		must(t, err)
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the grant never reached the service's cache: Watch didn't invalidate it")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 6. A service writes only its own namespace.
	if _, err := hz.UpsertResource(ctx, hayat.Ref("payments/payout", "p-1"), "Payout", hayat.Resource{NodeID: finance.ID.String()}, ""); err == nil {
		t.Fatal("billing wrote a payments resource")
	}

	// 7. A user's access token verifies offline; a service's doesn't pass as a user's.
	claims, err := hz.VerifyAccessToken(ctx, token, "")
	if err != nil || claims.Subject != admin.ID.String() || claims.SessionID == "" {
		t.Fatalf("user token: %v %+v", err, claims)
	}

	// 8. What each member was given, for a members page: Ben's own grant, Ana's
	//    through Finance, the owner's tenant — and, narrowed to Ben, only his.
	_, _, err = e.a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: ledgerNode.ID, PrincipalType: tree.PrincipalGroup, PrincipalID: finance.ID, RoleID: accountant}, "test")
	must(t, err)
	all, err := hz.Directory.ListTenantGrants(ctx, connect.NewRequest(&authzv1.ListTenantGrantsRequest{TenantId: tn.ID.String()}))
	must(t, err)
	got := map[string]string{}
	for _, g := range all.Msg.Grants {
		got[g.UserId+" "+g.Resource.Name+" "+g.Via] = g.RoleName + "|" + g.GroupName
	}
	for key, want := range map[string]string{
		ben.ID.String() + " Ledger 1 user":  "Accountant|",
		ana.ID.String() + " Ledger 1 group": "Accountant|Finance",
		admin.ID.String() + " Acme owner":   "|",
	} {
		if got[key] != want {
			t.Fatalf("ListTenantGrants: %q = %q, want %q (all: %v)", key, got[key], want, got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("ListTenantGrants: expected exactly three, got %v", got)
	}
	bens, err := hz.Directory.ListTenantGrants(ctx, connect.NewRequest(&authzv1.ListTenantGrantsRequest{TenantId: tn.ID.String(), UserIds: []string{ben.ID.String()}}))
	must(t, err)
	if len(bens.Msg.Grants) != 1 || bens.Msg.Grants[0].UserId != ben.ID.String() {
		t.Fatalf("narrowed to Ben: %v", bens.Msg.Grants)
	}

	// 9. Who reaches the ledger, for the console's tree: the tenant's owner,
	//    Ben's own grant, Finance's grant and placement for Ana, and a grant to
	//    everyone inherited from Finance. Ben's tenant grant stops at the tenant.
	_, _, err = e.a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: finance.ID, PrincipalType: tree.PrincipalGroup, PrincipalID: tenantNode.ID, RoleID: catalog.RoleGroupManager, Inherit: true}, "test")
	must(t, err)
	_, _, err = e.a.Tree.AddGrant(ctx, tree.GrantInput{NodeID: tenantNode.ID, PrincipalType: tree.PrincipalUser, PrincipalID: ben.ID, RoleID: catalog.RoleTenantAdmin}, "test")
	must(t, err)
	treeAdmin := adminv1connect.NewTreeAdminServiceClient(http.DefaultClient, e.admin, bearer(token))
	access, err := treeAdmin.ListNodeAccess(ctx, connect.NewRequest(&adminv1.ListNodeAccessRequest{NodeId: ledgerNode.ID.String()}))
	must(t, err)
	reach := map[string]bool{}
	for _, a := range access.Msg.Entries {
		reach[a.User.Email+" "+a.Via+" "+a.Source.Name+" "+a.RoleName+" "+a.GroupName] = true
	}
	want := []string{
		"root@example.com owner Acme  ",
		"ben@example.com user Ledger 1 Accountant ",
		"ana@example.com group Ledger 1 Accountant Finance",
		"ana@example.com placement Finance  Finance",
		"root@example.com everyone Finance Group manager ",
		"ana@example.com everyone Finance Group manager ",
		"ben@example.com everyone Finance Group manager ",
	}
	for _, key := range want {
		if !reach[key] {
			t.Fatalf("ListNodeAccess: missing %q (got %v)", key, reach)
		}
	}
	if len(access.Msg.Entries) != len(want) {
		t.Fatalf("ListNodeAccess: expected %d entries, got %v", len(want), reach)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestGRPCOverH2C calls the api role with the gRPC protocol over plain HTTP/2,
// the way a mesh or a gRPC client without TLS reaches it.
func TestGRPCOverH2C(t *testing.T) {
	e := newEnv(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	srv := &http.Server{Handler: e.a.APIHandler(), Protocols: apiserver.Protocols()}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	hc := &http.Client{Transport: &http.Transport{Protocols: protocols}}
	client := authzv1connect.NewAuthzServiceClient(hc, "http://"+l.Addr().String(), connect.WithGRPC())
	_, err = client.Check(context.Background(), connect.NewRequest(&authzv1.CheckRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("want a gRPC Unauthenticated answer over h2c, got %v", err)
	}
}

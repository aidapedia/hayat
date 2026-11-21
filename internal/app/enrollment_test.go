package app_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	authzv1 "github.com/aidapedia/hayat/gen/go/hayat/authz/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/authz/v1/authzv1connect"
	"github.com/aidapedia/hayat/internal/catalog"
	"github.com/aidapedia/hayat/internal/enrollment"
	"github.com/aidapedia/hayat/internal/tree"
)

const shopContract = `
service: shop
version: 1
permissions:
  - { key: instance.manage, scope: hayat/root }
default_roles:
  - { slug: admin, name: Shop admin, permissions: [instance.manage], applies_to: [hayat/root] }
`

// tokenWorks asks for a client-credentials token with these credentials.
func (e *env) tokenWorks(id, secret string) bool {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, e.auth+"/oauth/token", strings.NewReader(url.Values{"grant_type": {"client_credentials"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(id, secret)
	resp := e.must(http.DefaultClient.Do(req))
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// hasRootRole reports whether userID holds the role with this slug at the root.
func (e *env) hasRootRole(userEmail, slug string) bool {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.a.IDs.GetUserByEmail(ctx, userEmail)
	if err != nil {
		e.t.Fatal(err)
	}
	snap, err := e.a.Catalog.Snapshot(ctx, true)
	if err != nil {
		e.t.Fatal(err)
	}
	grants, err := e.a.Tree.Grants(ctx, catalog.RootID)
	if err != nil {
		e.t.Fatal(err)
	}
	return slices.ContainsFunc(grants, func(g tree.Grant) bool {
		return g.PrincipalType == tree.PrincipalUser && g.PrincipalID == u.ID && snap.Roles[g.RoleID].Slug == slug
	})
}

// A service's whole enrollment life: an instance admin makes a code for a
// name Hayat doesn't know, the deployment redeems it once and is registered;
// later an owner's code rotates its secret; codes are single use, revocable,
// expire, and give way to the next one.
func TestEnrollment(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	token, _ := e.signIn("root@example.com", "a long enough password")
	registry := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(token))
	api := authzv1connect.NewEnrollmentServiceClient(http.DefaultClient, e.api)
	enroll := func(in *authzv1.EnrollRequest) (*authzv1.EnrollResponse, error) {
		resp, err := api.Enroll(ctx, connect.NewRequest(in))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}
	mint := func(service, name string, ttl int32) (*adminv1.Enrollment, string) {
		t.Helper()
		resp, err := registry.CreateEnrollment(ctx, connect.NewRequest(&adminv1.CreateEnrollmentRequest{ServiceId: service, DisplayName: name, TtlSeconds: ttl}))
		if err != nil {
			t.Fatal(err)
		}
		return resp.Msg.Enrollment, resp.Msg.Code
	}
	refused := func(err error, want error) {
		t.Helper()
		if err == nil || code(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), want.Error()) {
			t.Fatalf("want %q, got %v", want, err)
		}
	}

	// 1. A code for a name not registered yet. Making another revokes the first.
	first, firstCode := mint("shop", "Shop", 0)
	if !strings.HasPrefix(firstCode, enrollment.Prefix) || first.Status != enrollment.StatusOpen ||
		first.ExpiresAt.AsTime().Sub(first.CreatedAt.AsTime()) != enrollment.DefaultTTL {
		t.Fatalf("first code: %v %q", first, firstCode)
	}
	_, shopCode := mint("shop", "Shop", 0)
	pending, err := registry.ListEnrollments(ctx, connect.NewRequest(&adminv1.ListEnrollmentsRequest{}))
	if err != nil || len(pending.Msg.Enrollments) != 2 || pending.Msg.Enrollments[1].Status != enrollment.StatusRevoked ||
		pending.Msg.Enrollments[0].Status != enrollment.StatusOpen || pending.Msg.Enrollments[0].CreatedBy.GetEmail() != "root@example.com" {
		t.Fatalf("codes for unregistered services: %v %v", pending, err)
	}
	_, err = enroll(&authzv1.EnrollRequest{Code: firstCode})
	refused(err, enrollment.ErrRevoked)
	_, err = enroll(&authzv1.EnrollRequest{Code: "hye_not-a-real-code"})
	refused(err, enrollment.ErrUnknown)
	if _, err := registry.CreateEnrollment(ctx, connect.NewRequest(&adminv1.CreateEnrollmentRequest{ServiceId: "shop", TtlSeconds: 10})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("a 10-second code: %v", err)
	}
	if _, err := registry.CreateEnrollment(ctx, connect.NewRequest(&adminv1.CreateEnrollmentRequest{ServiceId: "hayat"})); err == nil {
		t.Fatal("Hayat's own namespace can't be enrolled")
	}

	// 2. Inspecting shows what it would do, without spending it.
	look, err := api.InspectEnrollment(ctx, connect.NewRequest(&authzv1.InspectEnrollmentRequest{Code: shopCode}))
	if err != nil || look.Msg.ServiceId != "shop" || !look.Msg.Registers || look.Msg.MadeByEmail != "root@example.com" ||
		look.Msg.DisplayName != "Shop" || look.Msg.Issuer != e.auth {
		t.Fatalf("inspect: %v %v", look, err)
	}

	// 3. Anything wrong in the request is refused before anything is written,
	// and the code is given back.
	for _, bad := range []*authzv1.EnrollRequest{
		{Code: shopCode, Manifest: billingContract},
		{Code: shopCode, Manifest: shopContract, AdminRole: "hayat.admin"},
		{Code: shopCode, AppUrl: "shop.example.com"},
		{Code: shopCode, AppUrl: "https://shop.example.com/?next=x"},
		{Code: shopCode, AdminRole: "shop.admin", AdminEmail: "not an email"},
	} {
		if _, err := enroll(bad); err == nil {
			t.Fatalf("enrolled with %v", bad)
		}
	}
	if _, err := e.a.Catalog.GetService(ctx, "shop"); err == nil {
		t.Fatal("a refused request registered the service")
	}

	// A redemption in progress holds the code; a crashed one frees it later.
	must(t, exec(e, `UPDATE service_enrollments SET claimed_until = now() + interval '1 minute' WHERE service_id = 'shop' AND revoked_at IS NULL`))
	_, err = enroll(&authzv1.EnrollRequest{Code: shopCode})
	refused(err, enrollment.ErrBusy)
	must(t, exec(e, `UPDATE service_enrollments SET claimed_until = now() - interval '1 second' WHERE service_id = 'shop'`))

	// 4. Redeeming registers the service, owned by the code's maker, publishes
	// its contract, makes its sign-in client and grants the maker its admin role.
	reg, err := enroll(&authzv1.EnrollRequest{Code: shopCode, Manifest: shopContract, DisplayName: "Shop", Description: "Sells things",
		AppUrl: "https://shop.example.com/", AdminRole: "shop.admin"})
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Registered || reg.ServiceId != "shop" || reg.ClientId == "" || reg.ClientSecret == "" || reg.WebClientId == "" ||
		reg.ContractVersion != 1 || reg.ContractStatus != "active" || reg.Issuer != e.auth || len(reg.Warnings) != 0 {
		t.Fatalf("enroll: %+v", reg)
	}
	if !e.tokenWorks(reg.ClientId, reg.ClientSecret) {
		t.Fatal("the new credentials don't get a token")
	}
	web, err := e.a.Clients.Get(ctx, reg.WebClientId)
	if err != nil || web.Kind != "public" || !slices.Equal(web.Redirects, []string{"https://shop.example.com/callback"}) || web.ServiceID != "shop" {
		t.Fatalf("web client: %+v %v", web, err)
	}
	owners, _ := e.a.Catalog.ServiceOwners(ctx, "shop")
	root, _ := e.a.IDs.GetUserByEmail(ctx, "root@example.com")
	if len(owners) != 1 || owners[0] != root.ID {
		t.Fatalf("owners: %v", owners)
	}
	if !e.hasRootRole("root@example.com", "shop.admin") {
		t.Fatal("the code's maker didn't get the service's admin role")
	}
	_, err = enroll(&authzv1.EnrollRequest{Code: shopCode})
	refused(err, enrollment.ErrUsed)
	_, err = api.InspectEnrollment(ctx, connect.NewRequest(&authzv1.InspectEnrollmentRequest{Code: shopCode}))
	refused(err, enrollment.ErrUsed)

	// 5. Re-enrolling rotates the secret: the client id stays, the old secret
	// stops working, the sign-in client is kept, and nobody is re-granted.
	again, againCode := mint("shop", "", 600)
	if again.ExpiresAt.AsTime().Sub(again.CreatedAt.AsTime()) != 10*time.Minute {
		t.Fatalf("ttl: %v", again)
	}
	look, err = api.InspectEnrollment(ctx, connect.NewRequest(&authzv1.InspectEnrollmentRequest{Code: againCode}))
	if err != nil || look.Msg.Registers {
		t.Fatalf("inspect an existing service's code: %v %v", look, err)
	}
	rot, err := enroll(&authzv1.EnrollRequest{Code: againCode, Manifest: shopContract, AppUrl: "https://shop.example.com", AdminRole: "shop.admin",
		AdminEmail: "someone@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if rot.Registered || rot.ClientId != reg.ClientId || rot.ClientSecret == reg.ClientSecret || rot.WebClientId != reg.WebClientId ||
		rot.ContractVersion != 1 || rot.ContractStatus != "" {
		t.Fatalf("rotate: %+v", rot)
	}
	if e.tokenWorks(reg.ClientId, reg.ClientSecret) || !e.tokenWorks(rot.ClientId, rot.ClientSecret) {
		t.Fatal("rotation should retire the old secret and issue a working one")
	}
	if _, err := e.a.IDs.GetUserByEmail(ctx, "someone@example.com"); err == nil {
		t.Fatal("re-enrolling invited an admin")
	}

	// 6. The service moved: its sign-in client gains the new address.
	_, moveCode := mint("shop", "", 0)
	moved, err := enroll(&authzv1.EnrollRequest{Code: moveCode, AppUrl: "https://new-shop.example.com"})
	if err != nil || moved.WebClientId != reg.WebClientId {
		t.Fatalf("move: %+v %v", moved, err)
	}
	web, _ = e.a.Clients.Get(ctx, reg.WebClientId)
	if !slices.Equal(web.Redirects, []string{"https://shop.example.com/callback", "https://new-shop.example.com/callback"}) {
		t.Fatalf("redirects after a move: %v", web.Redirects)
	}

	// 7. Revoking: an open code goes off; a used one has nothing to revoke.
	open, openCode := mint("shop", "", 0)
	if _, err := registry.RevokeEnrollment(ctx, connect.NewRequest(&adminv1.RevokeEnrollmentRequest{Id: open.Id})); err != nil {
		t.Fatal(err)
	}
	_, err = enroll(&authzv1.EnrollRequest{Code: openCode})
	refused(err, enrollment.ErrRevoked)
	if _, err := registry.RevokeEnrollment(ctx, connect.NewRequest(&adminv1.RevokeEnrollmentRequest{Id: again.Id})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("revoking a used code: %v", err)
	}
	list, err := registry.ListEnrollments(ctx, connect.NewRequest(&adminv1.ListEnrollmentsRequest{ServiceId: "shop"}))
	if err != nil || len(list.Msg.Enrollments) != 5 {
		t.Fatalf("shop's codes: %v %v", list, err)
	}

	// 8. Expired.
	_, oldCode := mint("shop", "", 0)
	must(t, exec(e, `UPDATE service_enrollments SET expires_at = now() - interval '1 second' WHERE service_id = 'shop' AND used_at IS NULL AND revoked_at IS NULL`))
	_, err = enroll(&authzv1.EnrollRequest{Code: oldCode})
	refused(err, enrollment.ErrExpired)

	// 9. A new service whose admin is someone else: they're invited and granted.
	_, crmCode := mint("crm", "CRM", 0)
	crm, err := enroll(&authzv1.EnrollRequest{Code: crmCode, Manifest: strings.ReplaceAll(shopContract, "shop", "crm"), AdminRole: "crm.admin",
		AdminEmail: "dana@example.com"})
	if err != nil || !crm.Registered || crm.WebClientId != "" {
		t.Fatalf("crm: %+v %v", crm, err)
	}
	if !e.hasRootRole("dana@example.com", "crm.admin") || e.hasRootRole("root@example.com", "crm.admin") {
		t.Fatal("the named admin should get the role instead of the maker")
	}

	// 10. A role the contract doesn't define is a warning, not a failure.
	_, wikiCode := mint("wiki", "", 0)
	wiki, err := enroll(&authzv1.EnrollRequest{Code: wikiCode, AdminRole: "wiki.admin"})
	if err != nil || !wiki.Registered || len(wiki.Warnings) != 1 {
		t.Fatalf("wiki: %+v %v", wiki, err)
	}

	// 11. Whoever made the code must still be allowed when it's redeemed.
	_, ghostCode := mint("ghost", "", 0)
	must(t, exec(e, `UPDATE users SET status = 'disabled' WHERE email = 'root@example.com'`))
	_, err = enroll(&authzv1.EnrollRequest{Code: ghostCode})
	if err == nil || !strings.Contains(err.Error(), "can no longer sign in") {
		t.Fatalf("a disabled maker's code: %v", err)
	}
}

// Someone who isn't an instance admin can't make a code for a new name, or
// for a service they don't own; an owner can for theirs.
func TestEnrollmentAccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	rootToken, _ := e.signIn("root@example.com", "a long enough password")
	root := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(rootToken))
	if _, err := root.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "billing", OwnerEmail: "olga@example.com"})); err != nil {
		t.Fatal(err)
	}
	if _, err := root.RegisterService(ctx, connect.NewRequest(&adminv1.RegisterServiceRequest{Id: "payroll", OwnerEmail: "root@example.com"})); err != nil {
		t.Fatal(err)
	}
	olga := adminv1connect.NewRegistryServiceClient(http.DefaultClient, e.admin, bearer(e.activate("olga@example.com", "olga's long password")))

	if _, err := olga.CreateEnrollment(ctx, connect.NewRequest(&adminv1.CreateEnrollmentRequest{ServiceId: "newthing"})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("a non-admin enrolling a new name: %v", err)
	}
	if _, err := olga.CreateEnrollment(ctx, connect.NewRequest(&adminv1.CreateEnrollmentRequest{ServiceId: "payroll"})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("enrolling someone else's service: %v", err)
	}
	if _, err := olga.ListEnrollments(ctx, connect.NewRequest(&adminv1.ListEnrollmentsRequest{})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("listing codes for new names: %v", err)
	}
	made, err := olga.CreateEnrollment(ctx, connect.NewRequest(&adminv1.CreateEnrollmentRequest{ServiceId: "billing"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.RevokeEnrollment(ctx, connect.NewRequest(&adminv1.RevokeEnrollmentRequest{Id: made.Msg.Enrollment.Id})); err != nil {
		t.Fatalf("an instance admin revokes any code: %v", err)
	}

	// The CLI applies the same rule.
	if _, _, _, err := e.a.MintEnrollmentCode(ctx, "newthing", "", "olga@example.com", 0); err == nil {
		t.Fatal("the CLI let a non-admin enroll a new name")
	}
	if _, c, by, err := e.a.MintEnrollmentCode(ctx, "newthing", "", "", time.Hour); err != nil || by.Email != "root@example.com" || !strings.HasPrefix(c, enrollment.Prefix) {
		t.Fatalf("CLI default maker: %v %v", by.Email, err)
	}
}

func exec(e *env, sql string) error {
	_, err := e.a.Store.Pool.Exec(context.Background(), sql)
	return err
}

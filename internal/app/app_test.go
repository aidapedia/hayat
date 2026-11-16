package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	adminv1 "github.com/aidapedia/hayat/gen/go/hayat/admin/v1"
	"github.com/aidapedia/hayat/gen/go/hayat/admin/v1/adminv1connect"
	"github.com/aidapedia/hayat/internal/app"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/crypto"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/testdb"
)

const consoleURL = "http://console.test"

type env struct {
	t     *testing.T
	a     *app.App
	auth  string
	admin string
	api   string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	s := testdb.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var authH, adminH, apiH http.Handler
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { authH.ServeHTTP(w, r) }))
	adminSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { adminH.ServeHTTP(w, r) }))
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { apiH.ServeHTTP(w, r) }))
	t.Cleanup(authSrv.Close)
	t.Cleanup(adminSrv.Close)
	t.Cleanup(apiSrv.Close)

	cfg := config.Config{Region: "default", Issuer: authSrv.URL, ConsoleURL: consoleURL, MasterKey: make([]byte, 32),
		SessionTTL: time.Hour, AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Dev: true}
	a, err := app.New(cfg, s, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	h, cleanup, err := a.AuthHandler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	authH, adminH, apiH = h, a.AdminHandler(), a.APIHandler()
	return &env{t: t, a: a, auth: authSrv.URL, admin: adminSrv.URL, api: apiSrv.URL}
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// signIn runs the console's PKCE flow for these credentials and returns the
// access token and the browser (holding the Hayat session cookie).
func (e *env) signIn(email, password string) (string, *http.Client) {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	verifier := crypto.NewToken() + crypto.NewToken()
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"client_id": {hoidc.ConsoleClientID}, "redirect_uri": {consoleURL + "/callback"}, "response_type": {"code"},
		"scope": {"openid profile email"}, "state": {"s"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"}}

	resp := e.must(c.Get(e.auth + "/authorize?" + q.Encode()))
	login, _ := resp.Location()
	page := e.must(c.Get(e.auth + login.RequestURI()))
	body, _ := io.ReadAll(page.Body)
	csrf := csrfRe.FindSubmatch(body)
	if csrf == nil {
		e.t.Fatalf("no login form: %s", body)
	}
	resp = e.must(c.PostForm(e.auth+"/login", url.Values{"csrf": {string(csrf[1])},
		"authRequestID": {login.Query().Get("authRequestID")}, "email": {email}, "password": {password}}))
	var code string
	for i := 0; i < 5 && code == ""; i++ {
		loc, err := resp.Location()
		if err != nil {
			b, _ := io.ReadAll(resp.Body)
			e.t.Fatalf("sign-in stopped at %d: %s", resp.StatusCode, b)
		}
		if strings.HasPrefix(loc.String(), consoleURL) {
			code = loc.Query().Get("code")
			break
		}
		resp = e.must(c.Get(e.auth + loc.RequestURI()))
	}
	tok := e.must(http.PostForm(e.auth+"/oauth/token", url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {consoleURL + "/callback"}, "client_id": {hoidc.ConsoleClientID}, "code_verifier": {verifier}}))
	var out struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(tok.Body).Decode(&out)
	if out.AccessToken == "" {
		e.t.Fatal("no access token")
	}
	return out.AccessToken, c
}

func (e *env) must(resp *http.Response, err error) *http.Response {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
	return resp
}

func bearer(token string) connect.ClientOption {
	return connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if token != "" {
				req.Header().Set("Authorization", "Bearer "+token)
			}
			return next(ctx, req)
		}
	}))
}

func code(err error) connect.Code {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce.Code()
	}
	return connect.CodeUnknown
}

func TestAdminAPI(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.a.IDs.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}

	anon := adminv1connect.NewOverviewServiceClient(http.DefaultClient, e.admin, bearer(""))
	if _, err := anon.GetOverview(ctx, connect.NewRequest(&adminv1.GetOverviewRequest{})); code(err) != connect.CodeUnauthenticated {
		t.Fatalf("no token: want Unauthenticated, got %v", err)
	}

	token, browser := e.signIn("root@example.com", "a long enough password")
	overview := adminv1connect.NewOverviewServiceClient(http.DefaultClient, e.admin, bearer(token))
	ov, err := overview.GetOverview(ctx, connect.NewRequest(&adminv1.GetOverviewRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if ov.Msg.Users != 1 || ov.Msg.Clients != 1 {
		t.Fatalf("overview: %+v", ov.Msg)
	}

	// Admin creates a tenant whose owner is invited by email.
	tenants := adminv1connect.NewTenantServiceClient(http.DefaultClient, e.admin, bearer(token))
	created, err := tenants.CreateTenant(ctx, connect.NewRequest(&adminv1.CreateTenantRequest{Name: "Acme", OwnerEmail: "bea@example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	if created.Msg.Tenant.Slug != "acme" || created.Msg.Tenant.MemberCount != 1 {
		t.Fatalf("tenant: %+v", created.Msg.Tenant)
	}
	if _, err := tenants.CreateTenant(ctx, connect.NewRequest(&adminv1.CreateTenantRequest{Name: "Acme", OwnerEmail: "bea@example.com"})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("duplicate slug should be a user-facing error, got %v", err)
	}

	// A regular user can read their own account but not the admin services.
	bea, _ := e.a.IDs.GetUserByEmail(ctx, "bea@example.com")
	if err := e.a.IDs.ResendVerification(ctx, bea.ID); err != nil {
		t.Fatal(err)
	}
	region, link := e.latestLink(bea.Email)
	if _, err := e.a.IDs.CompleteVerification(ctx, region, link, "bea's long password"); err != nil {
		t.Fatal(err)
	}
	beaToken, _ := e.signIn("bea@example.com", "bea's long password")
	account := adminv1connect.NewAccountServiceClient(http.DefaultClient, e.admin, bearer(beaToken))
	me, err := account.GetMe(ctx, connect.NewRequest(&adminv1.GetMeRequest{}))
	if err != nil || me.Msg.User.Email != "bea@example.com" || len(me.Msg.Tenants) != 1 {
		t.Fatalf("GetMe: %v %v", me, err)
	}
	beaOverview := adminv1connect.NewOverviewServiceClient(http.DefaultClient, e.admin, bearer(beaToken))
	if _, err := beaOverview.GetOverview(ctx, connect.NewRequest(&adminv1.GetOverviewRequest{})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("non-admin: want PermissionDenied, got %v", err)
	}

	testTenantOwner(t, e, tenants, created.Msg.Tenant.Id, beaToken)
	testRoleGrants(t, e, token, beaToken, created.Msg.Tenant.Id, me.Msg.User.Id)

	// Signing out of Hayat cuts the console off at once, not when the token expires.
	page := e.must(browser.Get(e.auth + "/logout"))
	body, _ := io.ReadAll(page.Body)
	csrf := csrfRe.FindSubmatch(body)
	e.must(browser.PostForm(e.auth+"/logout", url.Values{"csrf": {string(csrf[1])}}))
	if _, err := overview.GetOverview(ctx, connect.NewRequest(&adminv1.GetOverviewRequest{})); code(err) != connect.CodeUnauthenticated {
		t.Fatalf("after sign-out: want Unauthenticated, got %v", err)
	}
}

// The roles page reads labels from the catalog and asks where a role is
// granted; only an instance admin may ask that, and a role in use can't go.
func testRoleGrants(t *testing.T, e *env, adminToken, beaToken, acmeID, beaID string) {
	ctx := context.Background()
	cat := adminv1connect.NewCatalogServiceClient(http.DefaultClient, e.admin, bearer(adminToken))
	catalogResp, err := cat.GetCatalog(ctx, connect.NewRequest(&adminv1.GetCatalogRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(catalogResp.Msg.Permissions, func(p *adminv1.PermissionInfo) bool { return p.Key == "hayat.members.manage" }); i < 0 || catalogResp.Msg.Permissions[i].Label != "Manage members" {
		t.Fatalf("platform permissions carry a label: %v", catalogResp.Msg.Permissions)
	}

	treeAdmin := adminv1connect.NewTreeAdminServiceClient(http.DefaultClient, e.admin, bearer(adminToken))
	tr, err := treeAdmin.GetTenantTree(ctx, connect.NewRequest(&adminv1.GetTenantTreeRequest{TenantId: acmeID}))
	if err != nil {
		t.Fatal(err)
	}
	var tenantNode *adminv1.Node
	for _, n := range tr.Msg.Nodes {
		if n.Type == "hayat/tenant" {
			tenantNode = n
		}
	}
	if tenantNode == nil {
		t.Fatal("tenant node missing from its tree")
	}
	group, err := treeAdmin.CreateGroup(ctx, connect.NewRequest(&adminv1.CreateGroupRequest{ParentId: tenantNode.Id, Name: "Engineering"}))
	if err != nil {
		t.Fatal(err)
	}
	role, err := cat.CreateRole(ctx, connect.NewRequest(&adminv1.CreateRoleRequest{Name: "Group helper", AppliesTo: []string{"hayat/group"}, Permissions: []string{"hayat.groups.manage"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := treeAdmin.AddGrant(ctx, connect.NewRequest(&adminv1.AddGrantRequest{NodeId: group.Msg.Node.Id, PrincipalType: "user", PrincipalId: beaID,
		RoleId: role.Msg.Role.Id, Inherit: true})); err != nil {
		t.Fatal(err)
	}

	got, err := cat.ListRoleGrants(ctx, connect.NewRequest(&adminv1.ListRoleGrantsRequest{RoleId: role.Msg.Role.Id}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Msg.Grants) != 1 || got.Msg.Truncated {
		t.Fatalf("one grant: %v", got.Msg)
	}
	g := got.Msg.Grants[0]
	if g.NodeName != "Engineering" || g.NodeType != "hayat/group" || g.TenantId != acmeID || !slices.Equal(g.Path, []string{tenantNode.Name}) ||
		g.Grant.PrincipalEmail != "bea@example.com" {
		t.Fatalf("grant place: %+v", g)
	}

	bea := adminv1connect.NewCatalogServiceClient(http.DefaultClient, e.admin, bearer(beaToken))
	if _, err := bea.ListRoleGrants(ctx, connect.NewRequest(&adminv1.ListRoleGrantsRequest{RoleId: role.Msg.Role.Id})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("a tenant owner can't list grants across tenants: %v", err)
	}
	if _, err := cat.DeleteRole(ctx, connect.NewRequest(&adminv1.DeleteRoleRequest{Id: role.Msg.Role.Id})); code(err) != connect.CodeFailedPrecondition {
		t.Fatalf("deleting a role in use: want FailedPrecondition, got %v", err)
	}
}

// activate signs an invited address in for the first time and returns its token.
func (e *env) activate(addr, password string) string {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.a.IDs.GetUserByEmail(ctx, addr)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.a.IDs.ResendVerification(ctx, u.ID); err != nil {
		e.t.Fatal(err)
	}
	region, link := e.latestLink(addr)
	if _, err := e.a.IDs.CompleteVerification(ctx, region, link, password); err != nil {
		e.t.Fatal(err)
	}
	token, _ := e.signIn(addr, password)
	return token
}

// A tenant's owner runs it from the console without being an instance admin:
// the tenant service is allowed by permission on the tenant node.
func testTenantOwner(t *testing.T, e *env, admin adminv1connect.TenantServiceClient, acmeID, beaToken string) {
	ctx := context.Background()
	other, err := admin.CreateTenant(ctx, connect.NewRequest(&adminv1.CreateTenantRequest{Name: "Globex", OwnerEmail: "root@example.com"}))
	if err != nil {
		t.Fatal(err)
	}

	bea := adminv1connect.NewTenantServiceClient(http.DefaultClient, e.admin, bearer(beaToken))
	list, err := bea.ListTenants(ctx, connect.NewRequest(&adminv1.ListTenantsRequest{}))
	if err != nil || len(list.Msg.Tenants) != 1 || list.Msg.Tenants[0].Id != acmeID {
		t.Fatalf("owner lists only their tenants: %v %v", list, err)
	}
	got, err := bea.GetTenant(ctx, connect.NewRequest(&adminv1.GetTenantRequest{Id: acmeID}))
	if err != nil || !slices.Contains(got.Msg.MyPermissions, "hayat.members.manage") {
		t.Fatalf("owner holds every permission on their tenant: %v %v", got, err)
	}
	if _, err := bea.GetTenant(ctx, connect.NewRequest(&adminv1.GetTenantRequest{Id: other.Msg.Tenant.Id})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("someone else's tenant: want PermissionDenied, got %v", err)
	}
	if _, err := bea.RenameTenant(ctx, connect.NewRequest(&adminv1.RenameTenantRequest{Id: acmeID, Name: "Acme Inc"})); err != nil {
		t.Fatal(err)
	}
	if _, err := bea.AddMember(ctx, connect.NewRequest(&adminv1.AddMemberRequest{TenantId: acmeID, Email: "carl@example.com"})); err != nil {
		t.Fatal(err)
	}
	tree := adminv1connect.NewTreeAdminServiceClient(http.DefaultClient, e.admin, bearer(beaToken))
	if _, err := tree.GetTenantTree(ctx, connect.NewRequest(&adminv1.GetTenantTreeRequest{TenantId: acmeID})); err != nil {
		t.Fatalf("owner opens their resource tree: %v", err)
	}
	for name, call := range map[string]func() error{
		"create": func() error {
			_, err := bea.CreateTenant(ctx, connect.NewRequest(&adminv1.CreateTenantRequest{Name: "Mine", OwnerEmail: "bea@example.com"}))
			return err
		},
		"delete": func() error {
			_, err := bea.DeleteTenant(ctx, connect.NewRequest(&adminv1.DeleteTenantRequest{Id: acmeID}))
			return err
		},
	} {
		if code(call()) != connect.CodePermissionDenied {
			t.Fatalf("owner %s: want PermissionDenied", name)
		}
	}

	// A plain member sees the tenant but changes nothing, and can't take ownership.
	carlToken := e.activate("carl@example.com", "carl's long password")
	carl := adminv1connect.NewTenantServiceClient(http.DefaultClient, e.admin, bearer(carlToken))
	seen, err := carl.GetTenant(ctx, connect.NewRequest(&adminv1.GetTenantRequest{Id: acmeID}))
	if err != nil || len(seen.Msg.MyPermissions) != 0 {
		t.Fatalf("member reads the tenant: %v %v", seen, err)
	}
	// The roster pages in join order: bea (the owner) first, then carl.
	first, err := carl.ListMembers(ctx, connect.NewRequest(&adminv1.ListMembersRequest{TenantId: acmeID, PageSize: 1}))
	if err != nil || len(first.Msg.Members) != 1 || !first.Msg.Members[0].IsOwner || first.Msg.NextPageToken == "" {
		t.Fatalf("first page of members: %v %v", first, err)
	}
	second, err := carl.ListMembers(ctx, connect.NewRequest(&adminv1.ListMembersRequest{TenantId: acmeID, PageSize: 1, PageToken: first.Msg.NextPageToken}))
	if err != nil || len(second.Msg.Members) != 1 || second.Msg.Members[0].User.Email != "carl@example.com" || second.Msg.NextPageToken != "" {
		t.Fatalf("second page of members: %v %v", second, err)
	}
	carlID := second.Msg.Members[0].User.Id
	if _, err := bea.ListMembers(ctx, connect.NewRequest(&adminv1.ListMembersRequest{TenantId: other.Msg.Tenant.Id})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("someone else's roster: want PermissionDenied, got %v", err)
	}
	for name, call := range map[string]func() error{
		"rename": func() error {
			_, err := carl.RenameTenant(ctx, connect.NewRequest(&adminv1.RenameTenantRequest{Id: acmeID, Name: "Carl's"}))
			return err
		},
		"add member": func() error {
			_, err := carl.AddMember(ctx, connect.NewRequest(&adminv1.AddMemberRequest{TenantId: acmeID, Email: "dee@example.com"}))
			return err
		},
		"take ownership": func() error {
			_, err := carl.TransferOwnership(ctx, connect.NewRequest(&adminv1.TransferOwnershipRequest{TenantId: acmeID, UserId: carlID}))
			return err
		},
	} {
		if code(call()) != connect.CodePermissionDenied {
			t.Fatalf("member %s: want PermissionDenied", name)
		}
	}

	if _, err := bea.RemoveMember(ctx, connect.NewRequest(&adminv1.RemoveMemberRequest{TenantId: acmeID, UserId: carlID})); err != nil {
		t.Fatal(err)
	}
	if _, err := carl.GetTenant(ctx, connect.NewRequest(&adminv1.GetTenantRequest{Id: acmeID})); code(err) != connect.CodePermissionDenied {
		t.Fatalf("after removal: want PermissionDenied, got %v", err)
	}
}

var linkRe = regexp.MustCompile(`/verify\?r=(\w+)&t=([\w-]+)`)

func (e *env) latestLink(addr string) (string, string) {
	e.t.Helper()
	var body string
	err := e.a.Store.Pool.QueryRow(context.Background(),
		`SELECT body_text FROM email_outbox WHERE to_addr = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, addr).Scan(&body)
	if err != nil {
		e.t.Fatal(err)
	}
	m := linkRe.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no link: %s", body)
	}
	return m[1], m[2]
}

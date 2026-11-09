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

	// Signing out of Hayat cuts the console off at once, not when the token expires.
	page := e.must(browser.Get(e.auth + "/logout"))
	body, _ := io.ReadAll(page.Body)
	csrf := csrfRe.FindSubmatch(body)
	e.must(browser.PostForm(e.auth+"/logout", url.Values{"csrf": {string(csrf[1])}}))
	if _, err := overview.GetOverview(ctx, connect.NewRequest(&adminv1.GetOverviewRequest{})); code(err) != connect.CodeUnauthenticated {
		t.Fatalf("after sign-out: want Unauthenticated, got %v", err)
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

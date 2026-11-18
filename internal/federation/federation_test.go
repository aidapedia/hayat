package federation_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/federation"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/tenancy"
	"github.com/aidapedia/hayat/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }

// fakeIdP plays Google, GitHub and a company OpenID Connect provider: codes
// map to the profile the next exchange returns.
type fakeIdP struct {
	mu       sync.Mutex
	srv      *httptest.Server
	profiles map[string]map[string]any // code → userinfo
	emails   map[string][]map[string]any
	verifier map[string]string // code → code_verifier received
}

func newFakeIdP(t *testing.T) *fakeIdP {
	f := &fakeIdP{profiles: map[string]map[string]any{}, emails: map[string][]map[string]any{}, verifier: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize",
			"token_endpoint": f.srv.URL + "/token", "userinfo_endpoint": f.srv.URL + "/userinfo"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := r.PostForm.Get("code")
		f.mu.Lock()
		f.verifier[code] = r.PostForm.Get("code_verifier")
		_, ok := f.profiles[code]
		f.mu.Unlock()
		if !ok || r.PostForm.Get("client_secret") != "shh" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-" + code, "token_type": "Bearer", "expires_in": 3600})
	})
	userinfo := func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer at-")
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.profiles[code])
	}
	mux.HandleFunc("/userinfo", userinfo)
	mux.HandleFunc("/user", userinfo)
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer at-")
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.emails[code])
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIdP) endpoints() federation.Endpoints {
	return federation.Endpoints{Auth: f.srv.URL + "/authorize", Token: f.srv.URL + "/token", UserInfo: f.srv.URL + "/userinfo", Emails: f.srv.URL + "/user/emails"}
}

type harness struct {
	t       *testing.T
	fed     *federation.Service
	ids     *identity.Service
	tenants *tenancy.Service
	idp     *fakeIdP
	txt     map[string][]string
}

func newHarness(t *testing.T) *harness {
	s := testdb.New(t)
	box, _ := crypto.NewBox(make([]byte, 32))
	ids := identity.New(s, "https://auth.example.com", time.Hour)
	tenants := tenancy.New(s)
	fed := federation.New(s, box, ids, tenants, "https://auth.example.com", true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := &harness{t: t, fed: fed, ids: ids, tenants: tenants, idp: newFakeIdP(t), txt: map[string][]string{}}
	fed.SetEndpoints(federation.KindGoogle, h.idp.endpoints())
	gh := h.idp.endpoints()
	gh.UserInfo = h.idp.srv.URL + "/user"
	fed.SetEndpoints(federation.KindGitHub, gh)
	fed.SetLookupTXT(func(_ context.Context, name string) ([]string, error) { return h.txt[name], nil })
	return h
}

// trip runs a sign-in through the fake provider and answers what Hayat decided.
func (h *harness) trip(p federation.Provider, code string, profile map[string]any, link *uuid.UUID) (identity.User, error) {
	h.t.Helper()
	ctx := context.Background()
	start, err := h.fed.Begin(ctx, p, "ar-1", link, "")
	if err != nil {
		h.t.Fatal(err)
	}
	u, _ := url.Parse(start.URL)
	if u.Query().Get("state") != start.State || u.Query().Get("code_challenge_method") != "S256" ||
		u.Query().Get("redirect_uri") != "https://auth.example.com/sso/callback" {
		h.t.Fatalf("authorize URL: %s", start.URL)
	}
	h.idp.mu.Lock()
	h.idp.profiles[code] = profile
	h.idp.mu.Unlock()
	r, err := h.fed.Complete(ctx, start.State, code)
	if err != nil {
		return identity.User{}, err
	}
	if r.AuthRequestID != "ar-1" {
		h.t.Fatalf("the trip remembers its sign-in, got %q", r.AuthRequestID)
	}
	if v := h.idp.verifier[code]; len(v) < 43 {
		h.t.Fatalf("the code is exchanged with its PKCE verifier, got %q", v)
	}
	if _, err := h.fed.Complete(ctx, start.State, code); !errors.Is(err, federation.ErrStateExpired) {
		h.t.Fatalf("a state works once, got %v", err)
	}
	return h.fed.SignIn(ctx, r)
}

func google(sub, email string, verified any) map[string]any {
	return map[string]any{"sub": sub, "email": email, "email_verified": verified, "name": "Dana Doe", "picture": "https://lh3.example/p.png"}
}

func TestGoogleSignsUpLinksAndMatches(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.fed.SaveInstanceProvider(ctx, federation.KindGoogle, federation.ProviderInput{ClientID: "cid"}, "test"); !errors.Is(err, federation.ErrNoSecret) {
		t.Fatalf("the first save needs the secret, got %v", err)
	}
	g, err := h.fed.SaveInstanceProvider(ctx, federation.KindGoogle, federation.ProviderInput{ClientID: "cid", ClientSecret: "shh", Enabled: true}, "test")
	if err != nil || !g.HasSecret || g.Name != "Google" {
		t.Fatalf("save: %+v %v", g, err)
	}

	// Someone new: an active account, verified by Google, linked, no password.
	dana, err := h.trip(g, "c1", google("g-1", "dana@example.com", true), nil)
	if err != nil || dana.Status != identity.StatusActive || !dana.EmailVerified || dana.HasPassword || dana.AvatarURL != "https://lh3.example/p.png" {
		t.Fatalf("new account: %+v %v", dana, err)
	}
	// The same Google account again is the same person, even with a new email.
	again, err := h.trip(g, "c2", google("g-1", "dana.new@example.com", true), nil)
	if err != nil || again.ID != dana.ID {
		t.Fatalf("a linked account is matched by its id, not its email: %+v %v", again, err)
	}

	// A pending account (invited, never set a password) is matched and activated.
	pending, _, _ := h.ids.Invite(ctx, "pat@example.com", "Pat", "test")
	pat, err := h.trip(g, "c3", google("g-2", "pat@example.com", "true"), nil)
	if err != nil || pat.ID != pending.ID || pat.Status != identity.StatusActive {
		t.Fatalf("pending account: %+v %v", pat, err)
	}

	// Never by email: an unverified address, or an instance admin's.
	if _, err := h.trip(g, "c4", google("g-3", "eve@example.com", false), nil); !errors.Is(err, federation.ErrNoEmail) {
		t.Fatalf("an unverified email must not sign in, got %v", err)
	}
	if _, err := h.ids.CreateAdmin(ctx, "root@example.com", "Root", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.trip(g, "c5", google("g-4", "root@example.com", true), nil); !errors.Is(err, federation.ErrAdminLink) {
		t.Fatalf("an admin is never matched by email, got %v", err)
	}

	// Linking from Account, and the last way in.
	if _, err := h.trip(g, "c6", google("g-1", "x@example.com", true), &pat.ID); !errors.Is(err, federation.ErrLinkedElsewhere) {
		t.Fatalf("an account linked to Dana can't be linked to Pat, got %v", err)
	}
	linked, _ := h.fed.Linked(ctx, dana)
	if len(linked) != 1 || linked[0].Provider.Name != "Google" {
		t.Fatalf("linked: %+v", linked)
	}
	if err := h.fed.Unlink(ctx, dana, linked[0].ID); !errors.Is(err, federation.ErrLastWayIn) {
		t.Fatalf("Dana has no password: Google is her only way in, got %v", err)
	}

	// A turned-off provider signs nobody in.
	if _, err := h.fed.SaveInstanceProvider(ctx, federation.KindGoogle, federation.ProviderInput{ClientID: "cid", Enabled: false}, "test"); err != nil {
		t.Fatal(err)
	}
	g, _ = h.fed.Provider(ctx, g.ID)
	if _, err := h.fed.Begin(ctx, g, "", nil, ""); !errors.Is(err, federation.ErrProviderOff) {
		t.Fatalf("turned off: %v", err)
	}
}

func TestGitHubUsesOnlyAVerifiedAddress(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	gh, err := h.fed.SaveInstanceProvider(ctx, federation.KindGitHub, federation.ProviderInput{ClientID: "cid", ClientSecret: "shh", Enabled: true}, "test")
	if err != nil {
		t.Fatal(err)
	}
	h.idp.emails["c1"] = []map[string]any{
		{"email": "octo@unverified.example", "primary": true, "verified": false},
		{"email": "octo@example.com", "primary": false, "verified": true},
	}
	u, err := h.trip(gh, "c1", map[string]any{"id": 583231, "login": "octocat", "name": "", "avatar_url": "https://avatars.example/o.png"}, nil)
	if err != nil || u.Email != "octo@example.com" || u.Name != "octocat" {
		t.Fatalf("GitHub: an unverified primary is skipped for a verified address: %+v %v", u, err)
	}
}

func TestCompanySignInNeedsAVerifiedDomain(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	owner, _ := h.ids.CreateActive(ctx, "olga@acme.com", "Olga", "a long enough password", false, "test")
	acme, err := h.tenants.Create(ctx, "Acme", "acme", owner.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := h.tenants.Create(ctx, "Other", "other", owner.ID, "test")

	if _, err := h.fed.SaveTenantProvider(ctx, acme.ID, nil, federation.ProviderInput{Name: "Acme SSO", Issuer: "ftp://nope", ClientID: "x", ClientSecret: "y"}, "test"); !errors.Is(err, federation.ErrIssuer) {
		t.Fatalf("a bad issuer: %v", err)
	}
	okta, err := h.fed.SaveTenantProvider(ctx, acme.ID, nil, federation.ProviderInput{Name: "Acme SSO", Issuer: h.idp.srv.URL, ClientID: "cid", ClientSecret: "shh", Enabled: true}, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Domains: free mail is refused, an unverified claim gives way, a verified one doesn't.
	if _, err := h.fed.AddDomain(ctx, acme.ID, "gmail.com", "test"); err == nil {
		t.Fatal("nobody can claim gmail.com")
	}
	if _, err := h.fed.AddDomain(ctx, other.ID, "acme.com", "test"); err != nil {
		t.Fatal(err)
	}
	d, err := h.fed.AddDomain(ctx, acme.ID, "@ACME.com", "test")
	if err != nil || d.Domain != "acme.com" || d.TenantID != acme.ID || d.Verified() {
		t.Fatalf("Acme takes over an unverified claim: %+v %v", d, err)
	}

	// Unverified: Acme's provider can't sign anyone in at acme.com yet.
	ann := map[string]any{"sub": "okta-1", "email": "ann@acme.com", "email_verified": true, "name": "Ann"}
	if _, err := h.trip(okta, "c1", ann, nil); !errors.Is(err, federation.ErrNotYourDomain) {
		t.Fatalf("an unverified domain: %v", err)
	}
	if _, err := h.fed.VerifyDomain(ctx, acme.ID, "acme.com", "test"); !errors.Is(err, federation.ErrNotVerified) {
		t.Fatalf("no TXT record yet: %v", err)
	}
	h.txt[d.TXTName()] = []string{"something else", d.TXTValue()}
	if d, err = h.fed.VerifyDomain(ctx, acme.ID, "acme.com", "test"); err != nil || !d.Verified() {
		t.Fatalf("verify: %+v %v", d, err)
	}
	if _, err := h.fed.AddDomain(ctx, other.ID, "acme.com", "test"); !errors.Is(err, federation.ErrDomainClaimed) {
		t.Fatalf("a verified domain stays Acme's: %v", err)
	}
	if _, err := h.fed.SetDomainSignIn(ctx, acme.ID, "acme.com", &okta.ID, federation.PolicyOffer, "test"); err != nil {
		t.Fatal(err)
	}

	// Verified, but the provider doesn't create accounts: ask for an invite.
	if _, err := h.trip(okta, "c2", ann, nil); !errors.Is(err, federation.ErrAskForInvite) {
		t.Fatalf("no JIT: %v", err)
	}
	okta, _ = h.fed.SaveTenantProvider(ctx, acme.ID, &okta.ID, federation.ProviderInput{Name: "Acme SSO", Issuer: h.idp.srv.URL, ClientID: "cid", Enabled: true, JIT: true}, "test")
	u, err := h.trip(okta, "c3", ann, nil)
	if err != nil || u.Email != "ann@acme.com" {
		t.Fatalf("JIT: %+v %v", u, err)
	}
	if member, _ := h.tenants.IsMember(ctx, acme.ID, u.ID); !member {
		t.Fatal("someone JIT created joins the tenant")
	}
	// Another domain's address can't come through Acme's provider.
	if _, err := h.trip(okta, "c4", map[string]any{"sub": "okta-2", "email": "spy@evil.com", "email_verified": true}, nil); !errors.Is(err, federation.ErrNotYourDomain) {
		t.Fatalf("a foreign domain through Acme's provider: %v", err)
	}

	// Enforcing: Ann may only use Acme SSO; an instance admin at acme.com never is.
	if _, err := h.fed.SetDomainSignIn(ctx, acme.ID, "acme.com", nil, federation.PolicyEnforce, "test"); !errors.Is(err, federation.ErrEnforceNeeds) {
		t.Fatalf("enforcing needs a provider: %v", err)
	}
	if _, err := h.fed.SetDomainSignIn(ctx, acme.ID, "acme.com", &okta.ID, federation.PolicyEnforce, "test"); err != nil {
		t.Fatal(err)
	}
	if p, enforced, _ := h.fed.Enforced(ctx, "ann@acme.com"); !enforced || p.ID != okta.ID {
		t.Fatal("ann@acme.com must sign in with Acme SSO")
	}
	if _, err := h.ids.CreateAdmin(ctx, "it@acme.com", "IT", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	if _, enforced, _ := h.fed.Enforced(ctx, "it@acme.com"); enforced {
		t.Fatal("an instance admin keeps their password")
	}
	if err := h.fed.DeleteTenantProvider(ctx, acme.ID, okta.ID, "test"); !errors.Is(err, federation.ErrProviderInUse) {
		t.Fatalf("a provider a domain enforces can't be removed: %v", err)
	}
}

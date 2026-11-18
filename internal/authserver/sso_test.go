package authserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aidapedia/hayat/internal/federation"
)

// fakeGoogle answers every code with one profile.
func fakeGoogle(t *testing.T, profile map[string]any) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(profile) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestContinueWithGoogle(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	idp := fakeGoogle(t, map[string]any{"sub": "g-7", "email": "dana@example.com", "email_verified": true, "name": "Dana"})
	h.fed.SetEndpoints(federation.KindGoogle, federation.Endpoints{Auth: idp.URL + "/auth", Token: idp.URL + "/token", UserInfo: idp.URL + "/userinfo"})
	g, err := h.fed.SaveInstanceProvider(ctx, federation.KindGoogle, federation.ProviderInput{ClientID: "cid", ClientSecret: "shh", Enabled: true}, "test")
	if err != nil {
		t.Fatal(err)
	}

	c := browser()
	_, challenge := pkce()
	login := location(t, h.authorize(c, challenge, nil))
	page := body(t, h.get(c, login.String()))
	start := "/sso/start/" + g.ID.String() + "?authRequestID=" + login.Query().Get("authRequestID")
	if !strings.Contains(page, "Continue with Google") || !strings.Contains(page, strings.ReplaceAll(start, "&", "&amp;")) {
		t.Fatalf("the login page offers Google for this sign-in:\n%s", page)
	}

	toGoogle := location(t, h.get(c, h.url+start))
	if !strings.HasPrefix(toGoogle.String(), idp.URL+"/auth") {
		t.Fatalf("Hayat sends the browser to Google, got %s", toGoogle)
	}
	state := toGoogle.Query().Get("state")
	callback := h.url + "/sso/callback?code=abc&state=" + url.QueryEscape(state)

	// The same callback opened in another browser signs nobody in.
	if resp := h.get(browser(), callback); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a callback without the starting browser's cookie: %d", resp.StatusCode)
	}
	if code := h.followToClient(c, h.get(c, callback)); code == "" {
		t.Fatal("a Google sign-in ends with a code for the client")
	}
	if _, err := h.ids().GetUserByEmail(ctx, "dana@example.com"); err != nil {
		t.Fatalf("Dana's account was created from Google: %v", err)
	}
}

func TestEnforcedDomainSkipsThePassword(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/auth",
			"token_endpoint": "http://" + r.Host + "/token", "userinfo_endpoint": "http://" + r.Host + "/userinfo"})
	}))
	defer idp.Close()

	ann, err := h.ids().CreateActive(ctx, "ann@acme.com", "Ann", "a long enough password", false, "test")
	if err != nil {
		t.Fatal(err)
	}
	acme, err := h.tenants().Create(ctx, "Acme", "acme", ann.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.fed.SaveTenantProvider(ctx, acme.ID, nil, federation.ProviderInput{Name: "Acme SSO", Issuer: idp.URL, ClientID: "cid", ClientSecret: "shh", Enabled: true}, "test")
	if err != nil {
		t.Fatal(err)
	}
	d, _ := h.fed.AddDomain(ctx, acme.ID, "acme.com", "test")
	h.fed.SetLookupTXT(func(context.Context, string) ([]string, error) { return []string{d.TXTValue()}, nil })
	if _, err := h.fed.VerifyDomain(ctx, acme.ID, "acme.com", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.fed.SetDomainSignIn(ctx, acme.ID, "acme.com", &p.ID, federation.PolicyEnforce, "test"); err != nil {
		t.Fatal(err)
	}

	c := browser()
	_, challenge := pkce()
	login := location(t, h.authorize(c, challenge, nil))
	page := body(t, h.get(c, login.String()))
	if !strings.Contains(page, "Continue with company SSO") {
		t.Fatal("company SSO is offered once a tenant has it ready")
	}
	m := csrfRe.FindStringSubmatch(page)
	resp, err := c.PostForm(h.url+"/login", url.Values{"csrf": {m[1]}, "authRequestID": {login.Query().Get("authRequestID")},
		"email": {"ann@acme.com"}, "password": {"a long enough password"}})
	if err != nil {
		t.Fatal(err)
	}
	if to := location(t, resp); !strings.HasPrefix(to.String(), idp.URL+"/auth") || to.Query().Get("login_hint") != "ann@acme.com" {
		t.Fatalf("acme.com forces its own sign-in, even with the right password: %s", to)
	}
}

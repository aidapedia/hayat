package authserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

	jose "github.com/go-jose/go-jose/v4"

	"github.com/aidapedia/hayat/internal/authserver"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/identity"
	hoidc "github.com/aidapedia/hayat/internal/oidc"
	"github.com/aidapedia/hayat/internal/ratelimit"
	"github.com/aidapedia/hayat/internal/testdb"
)

const redirectURI = "http://console.test/callback"

type harness struct {
	t   *testing.T
	url string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	s := testdb.New(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var handler http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)

	key := make([]byte, 32)
	cfg := config.Config{Region: "default", Issuer: srv.URL, ConsoleURL: "http://console.test", MasterKey: key,
		SessionTTL: time.Hour, AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: time.Hour, Dev: true}
	box, _ := crypto.NewBox(key)
	ids := identity.New(s, cfg.Issuer, cfg.SessionTTL)
	clients := hoidc.NewClients(s, cfg.Issuer, cfg.Dev)
	keys := hoidc.NewKeyRing(s, box)
	if err := keys.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if err := clients.EnsureConsole(ctx, redirectURI, cfg.ConsoleURL); err != nil {
		t.Fatal(err)
	}
	storage := hoidc.NewStorage(s, ids, clients, keys, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	provider, err := hoidc.NewProvider(cfg.Issuer, storage, key, true, log)
	if err != nil {
		t.Fatal(err)
	}
	as, err := authserver.New(cfg, s, ids, storage, clients, provider, ratelimit.NewMemory(), log)
	if err != nil {
		t.Fatal(err)
	}
	handler = as.Handler()

	if _, err := ids.CreateAdmin(ctx, "ana@example.com", "Ana", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, url: srv.URL}
}

func browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func pkce() (verifier, challenge string) {
	verifier = crypto.NewToken() + crypto.NewToken()
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func (h *harness) authorize(c *http.Client, challenge string, extra url.Values) *http.Response {
	q := url.Values{
		"client_id":     {hoidc.ConsoleClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"openid profile email offline_access"},
		"state":         {"st4te"},
		"nonce":         {"n0nce"},
	}
	if challenge != "" {
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", "S256")
	}
	for k, v := range extra {
		q[k] = v
	}
	return h.get(c, h.url+"/authorize?"+q.Encode())
}

func (h *harness) get(c *http.Client, u string) *http.Response {
	h.t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

func location(t *testing.T, resp *http.Response) *url.URL {
	t.Helper()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want a redirect, got %d: %s", resp.StatusCode, body)
	}
	u, err := resp.Location()
	if err != nil {
		t.Fatal(err)
	}
	return u
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// signIn walks the login page and returns the code the client receives.
func (h *harness) signIn(c *http.Client, challenge string) string {
	h.t.Helper()
	login := location(h.t, h.authorize(c, challenge, nil))
	if login.Path != "/login" {
		h.t.Fatalf("authorize should send the browser to /login, got %s", login)
	}
	page := h.get(c, login.String())
	if page.StatusCode == http.StatusFound {
		return h.followToClient(c, page) // already signed in: single sign-on
	}
	body, _ := io.ReadAll(page.Body)
	m := csrfRe.FindSubmatch(body)
	if m == nil {
		h.t.Fatalf("no csrf field on the login page: %s", body)
	}
	resp, err := c.PostForm(h.url+"/login", url.Values{
		"csrf": {string(m[1])}, "authRequestID": {login.Query().Get("authRequestID")},
		"email": {"ana@example.com"}, "password": {"correct horse battery"},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return h.followToClient(c, resp)
}

// followToClient follows Hayat's own redirects until the browser would leave
// for the client, and returns the code.
func (h *harness) followToClient(c *http.Client, resp *http.Response) string {
	h.t.Helper()
	for i := 0; i < 5; i++ {
		u := location(h.t, resp)
		if strings.HasPrefix(u.String(), redirectURI) {
			if u.Query().Get("state") != "st4te" {
				h.t.Fatalf("state not returned: %s", u)
			}
			if e := u.Query().Get("error"); e != "" {
				h.t.Fatalf("client got an error: %s", u)
			}
			return u.Query().Get("code")
		}
		if !u.IsAbs() {
			u = mustParse(h.url + u.String())
		}
		resp = h.get(c, u.String())
	}
	h.t.Fatal("too many redirects")
	return ""
}

func mustParse(s string) *url.URL { u, _ := url.Parse(s); return u }

type tokens struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
}

func (h *harness) token(form url.Values) (int, tokens) {
	h.t.Helper()
	form.Set("client_id", hoidc.ConsoleClientID)
	resp, err := http.PostForm(h.url+"/oauth/token", form)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out tokens
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// verify checks a JWT against the published JWKS and returns its claims.
func (h *harness) verify(raw string) map[string]any {
	h.t.Helper()
	resp := h.get(http.DefaultClient, h.url+"/keys")
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		h.t.Fatal(err)
	}
	sig, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.ES256})
	if err != nil {
		h.t.Fatal(err)
	}
	keys := set.Key(sig.Signatures[0].Header.KeyID)
	if len(keys) == 0 {
		h.t.Fatal("token signed with a key that isn't in the JWKS")
	}
	payload, err := sig.Verify(keys[0].Key)
	if err != nil {
		h.t.Fatalf("signature: %v", err)
	}
	var claims map[string]any
	_ = json.Unmarshal(payload, &claims)
	return claims
}

func TestAuthorizationCodeFlow(t *testing.T) {
	h := newHarness(t)
	c := browser()
	verifier, challenge := pkce()
	code := h.signIn(c, challenge)

	// A wrong verifier must not redeem the code.
	if status, _ := h.token(url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {redirectURI}, "code_verifier": {"not-the-verifier-not-the-verifier-not-the-verifier"}}); status == http.StatusOK {
		t.Fatal("code redeemed with the wrong PKCE verifier")
	}

	code = h.signIn(c, challenge)
	status, tok := h.token(url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier}})
	if status != http.StatusOK || tok.AccessToken == "" || tok.IDToken == "" || tok.RefreshToken == "" {
		t.Fatalf("token exchange: %d %+v", status, tok)
	}
	if status, _ := h.token(url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier}}); status == http.StatusOK {
		t.Fatal("a code must work once")
	}

	id := h.verify(tok.IDToken)
	if id["iss"] != h.url || id["nonce"] != "n0nce" || id["email"] != "ana@example.com" || id["name"] != "Ana" {
		t.Fatalf("unexpected ID token claims: %v", id)
	}
	access := h.verify(tok.AccessToken)
	if access["sub"] != id["sub"] || access["sid"] == nil {
		t.Fatalf("access token must carry sub and sid: %v", access)
	}

	// Refresh rotates; reusing the old token revokes the whole family.
	status, next := h.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok.RefreshToken}})
	if status != http.StatusOK || next.RefreshToken == "" || next.RefreshToken == tok.RefreshToken {
		t.Fatalf("refresh: %d %+v", status, next)
	}
	if status, _ := h.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok.RefreshToken}}); status == http.StatusOK {
		t.Fatal("a rotated refresh token was accepted again")
	}
	if status, _ := h.token(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {next.RefreshToken}}); status == http.StatusOK {
		t.Fatal("reusing a rotated token must revoke its family")
	}
}

func TestSingleSignOnSkipsTheForm(t *testing.T) {
	h := newHarness(t)
	c := browser()
	_, challenge := pkce()
	h.signIn(c, challenge)

	// Same browser, second sign-in: no form, straight back to the client.
	resp := h.authorize(c, challenge, nil)
	login := location(t, resp)
	code := h.followToClient(c, h.get(c, login.String()))
	if code == "" {
		t.Fatal("an existing session should complete sign-in without a prompt")
	}
}

func TestPromptNoneWithoutSession(t *testing.T) {
	h := newHarness(t)
	c := browser()
	_, challenge := pkce()
	login := location(t, h.authorize(c, challenge, url.Values{"prompt": {"none"}}))
	if login.Path == "/login" {
		login = location(t, h.get(c, login.String()))
	}
	if !strings.HasPrefix(login.String(), redirectURI) || login.Query().Get("error") != "login_required" {
		t.Fatalf("prompt=none without a session must return login_required, got %s", login)
	}
}

func TestPublicClientNeedsPKCE(t *testing.T) {
	h := newHarness(t)
	resp := h.authorize(browser(), "", nil)
	u := location(t, resp)
	if u.Path == "/login" {
		t.Fatal("a public client got a sign-in without PKCE")
	}
}

func TestDiscovery(t *testing.T) {
	h := newHarness(t)
	resp := h.get(http.DefaultClient, h.url+"/.well-known/openid-configuration")
	var doc map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&doc)
	if doc["issuer"] != h.url || doc["jwks_uri"] == nil {
		t.Fatalf("discovery: %v", doc)
	}
}

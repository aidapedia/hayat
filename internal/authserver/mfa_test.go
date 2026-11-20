package authserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aidapedia/hayat/internal/authserver"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/testkey"
)

// ---------- helpers ----------

func (h *harness) user(email string) identity.User {
	h.t.Helper()
	u, err := h.ids().GetUserByEmail(context.Background(), email)
	if err != nil {
		h.t.Fatal(err)
	}
	return u
}

// addTOTP sets up an authenticator app for email and returns its secret.
func (h *harness) addTOTP(email string) string {
	h.t.Helper()
	ctx := context.Background()
	u := h.user(email)
	setup, err := h.ids().BeginTOTP(ctx, u)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.ids().ConfirmTOTP(ctx, u.ID, setup.FactorID, identity.TOTPCode(setup.Secret, time.Now().Add(-30*time.Second)), "test"); err != nil {
		h.t.Fatal(err)
	}
	return setup.Secret
}

// addPasskey registers a software passkey for email.
func (h *harness) addPasskey(email string) *testkey.Key {
	h.t.Helper()
	ctx := context.Background()
	u := h.user(email)
	key := testkey.New(h.url)
	opts, state, err := h.ids().BeginPasskeyRegistration(ctx, u)
	if err != nil {
		h.t.Fatal(err)
	}
	resp, err := key.Register(opts)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.ids().FinishPasskeyRegistration(ctx, u, state, resp, "Test key", "test"); err != nil {
		h.t.Fatal(err)
	}
	return key
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func csrfOf(t *testing.T, body string) string {
	t.Helper()
	m := csrfRe.FindStringSubmatch(body)
	if m == nil {
		m = dataCSRFRe.FindStringSubmatch(body) // a page with only passkey buttons
	}
	if m == nil {
		t.Fatalf("no csrf on the page: %s", body)
	}
	return m[1]
}

// loginWithPassword starts an authorization and submits the password; it
// returns the response to the login form and the auth request id.
func (h *harness) loginWithPassword(c *http.Client, email, password string) (*http.Response, string) {
	h.t.Helper()
	_, challenge := pkce()
	login := location(h.t, h.authorize(c, challenge, nil))
	body := readBody(h.t, h.get(c, login.String()))
	id := login.Query().Get("authRequestID")
	resp, err := c.PostForm(h.url+"/login", url.Values{"csrf": {csrfOf(h.t, body)}, "authRequestID": {id},
		"email": {email}, "password": {password}})
	if err != nil {
		h.t.Fatal(err)
	}
	return resp, id
}

func (h *harness) post(c *http.Client, path string, form url.Values) *http.Response {
	h.t.Helper()
	resp, err := c.PostForm(h.url+path, form)
	if err != nil {
		h.t.Fatal(err)
	}
	return resp
}

// postJSON calls a passkey endpoint the way hayat.js does.
func (h *harness) postJSON(c *http.Client, path, csrf string, body any) (int, []byte) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, h.url+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := c.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// passkeyCeremony runs begin → the device → finish and returns the redirect.
func (h *harness) passkeyCeremony(c *http.Client, csrf string, key *testkey.Key, body map[string]any) string {
	h.t.Helper()
	status, opts := h.postJSON(c, "/passkey/begin", csrf, body)
	if status != http.StatusOK {
		h.t.Fatalf("begin: %d %s", status, opts)
	}
	var cred []byte
	var err error
	if body["purpose"] == "register" {
		cred, err = key.Register(opts)
	} else {
		cred, err = key.Sign(opts)
	}
	if err != nil {
		h.t.Fatal(err)
	}
	body["credential"] = json.RawMessage(cred)
	status, out := h.postJSON(c, "/passkey/finish", csrf, body)
	if status != http.StatusOK {
		h.t.Fatalf("finish: %d %s", status, out)
	}
	var r struct{ Redirect string }
	_ = json.Unmarshal(out, &r)
	return r.Redirect
}

// latestAMR is how the newest session was signed in: what its tokens' amr carries.
func latestAMR(t *testing.T, h *harness) []any {
	t.Helper()
	var amr []string
	if err := h.store.Pool.QueryRow(context.Background(),
		`SELECT amr FROM sessions ORDER BY created_at DESC LIMIT 1`).Scan(&amr); err != nil {
		t.Fatal(err)
	}
	out := make([]any, len(amr))
	for i, a := range amr {
		out[i] = a
	}
	return out
}

func hasAll(list []any, want ...string) bool {
	for _, w := range want {
		found := false
		for _, v := range list {
			if v == w {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ---------- tests ----------

func TestPasswordThenAuthenticatorCode(t *testing.T) {
	h := newHarness(t)
	secret := h.addTOTP("ana@example.com")
	c := browser()

	resp, _ := h.loginWithPassword(c, "ana@example.com", "correct horse battery")
	if u := location(t, resp); u.Path != "/mfa" {
		t.Fatalf("a password alone must not sign in someone with an app, went to %s", u)
	}
	// No session yet: the console's own login page would still show.
	if got := location(t, h.get(c, h.url+"/reauth")); got.Path != "/login" {
		t.Fatalf("a half-done sign-in is not a session: %s", got)
	}
	page := readBody(t, h.get(c, h.url+"/mfa"))
	if !strings.Contains(page, "authenticator app") {
		t.Fatalf("the code field: %s", page)
	}
	csrf := csrfOf(t, page)
	bad := h.post(c, "/mfa", url.Values{"csrf": {csrf}, "code": {"12345x"}})
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a wrong code: %d", bad.StatusCode)
	}
	resp = h.post(c, "/mfa", url.Values{"csrf": {csrf}, "code": {identity.TOTPCode(secret, time.Now())}})
	code := h.followToClient(c, resp)
	if code == "" {
		t.Fatal("no code for the client")
	}
	if amr := latestAMR(t, h); !hasAll(amr, "pwd", "otp", "mfa") {
		t.Fatalf("amr: %v", amr)
	}
}

func TestTooManyWrongCodesEndTheChallenge(t *testing.T) {
	h := newHarness(t)
	h.addTOTP("ana@example.com")
	c := browser()
	resp, _ := h.loginWithPassword(c, "ana@example.com", "correct horse battery")
	location(t, resp)
	csrf := csrfOf(t, readBody(t, h.get(c, h.url+"/mfa")))
	for i := 0; i < identity.MaxChallengeAttempts; i++ {
		h.post(c, "/mfa", url.Values{"csrf": {csrf}, "code": {"000000"}}).Body.Close()
	}
	if resp := h.get(c, h.url+"/mfa"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("the challenge should be gone after %d wrong codes: %d", identity.MaxChallengeAttempts, resp.StatusCode)
	}
}

func TestRecoveryCodeInsteadOfTheApp(t *testing.T) {
	h := newHarness(t)
	h.addTOTP("ana@example.com")
	codes, err := h.ids().NewRecoveryCodes(context.Background(), h.user("ana@example.com").ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	c := browser()
	resp, _ := h.loginWithPassword(c, "ana@example.com", "correct horse battery")
	location(t, resp)
	csrf := csrfOf(t, readBody(t, h.get(c, h.url+"/mfa")))
	if code := h.followToClient(c, h.post(c, "/mfa", url.Values{"csrf": {csrf}, "code": {codes[0]}})); code == "" {
		t.Fatal("a recovery code should sign in")
	}
}

func TestSignInWithAPasskey(t *testing.T) {
	h := newHarness(t)
	key := h.addPasskey("ana@example.com")
	c := browser()
	_, challenge := pkce()
	login := location(t, h.authorize(c, challenge, nil))
	page := readBody(t, h.get(c, login.String()))
	if !strings.Contains(page, `data-passkey="login"`) {
		t.Fatal("the login page offers a passkey")
	}
	to := h.passkeyCeremony(c, csrfOf(t, page), key, map[string]any{"purpose": "login", "authRequestID": login.Query().Get("authRequestID")})
	resp := h.get(c, to)
	if code := h.followToClient(c, resp); code == "" {
		t.Fatal("no code")
	}
	if amr := latestAMR(t, h); !hasAll(amr, "hwk", "mfa") {
		t.Fatalf("a passkey is two factors: %v", amr)
	}
}

func TestPasskeyEndpointsNeedTheCSRFHeader(t *testing.T) {
	h := newHarness(t)
	c := browser()
	readBody(t, h.get(c, h.url+"/login")) // sets the csrf cookie
	if status, _ := h.postJSON(c, "/passkey/begin", "forged", map[string]any{"purpose": "login"}); status != http.StatusForbidden {
		t.Fatalf("no csrf: %d", status)
	}
}

func TestPasskeyAsTheSecondFactor(t *testing.T) {
	h := newHarness(t)
	key := h.addPasskey("ana@example.com")
	c := browser()
	resp, id := h.loginWithPassword(c, "ana@example.com", "correct horse battery")
	if u := location(t, resp); u.Path != "/mfa" {
		t.Fatalf("a passkey is a second factor after a password: %s", u)
	}
	page := readBody(t, h.get(c, h.url+"/mfa"))
	if !strings.Contains(page, `data-passkey="mfa"`) || strings.Contains(page, "authenticator app") {
		t.Fatalf("passkey only: %s", page)
	}
	to := h.passkeyCeremony(c, csrfOf(t, page), key, map[string]any{"purpose": "mfa", "authRequestID": id})
	if code := h.followToClient(c, h.get(c, to)); code == "" {
		t.Fatal("no code")
	}
	if amr := latestAMR(t, h); !hasAll(amr, "pwd", "hwk", "mfa") {
		t.Fatalf("amr: %v", amr)
	}
}

var (
	dataCSRFRe = regexp.MustCompile(`data-csrf="([^"]+)"`)
	factorRe   = regexp.MustCompile(`name="factor" value="([^"]+)"`)
	secretRe   = regexp.MustCompile(`<code>([A-Z2-7 ]+)</code>`)
)

func TestTenantRequirementSetsUpAnAppBeforeSigningIn(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	ana := h.user("ana@example.com")
	tn, err := h.tenants().Create(ctx, "Strict", "strict", ana.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.tenants().SetRequireMFA(ctx, tn.ID, true, "test"); err != nil {
		t.Fatal(err)
	}
	c := browser()
	resp, _ := h.loginWithPassword(c, "ana@example.com", "correct horse battery")
	if u := location(t, resp); u.Path != "/mfa/setup" {
		t.Fatalf("a member of a strict tenant sets up a factor first, went to %s", u)
	}
	if page := readBody(t, h.get(c, h.url+"/mfa/setup")); !strings.Contains(page, "Strict requires two-step verification") {
		t.Fatalf("say who requires it: %s", page)
	}
	page := readBody(t, h.get(c, h.url+"/mfa/setup/app"))
	if !strings.Contains(page, `src="data:image/png;base64,`) {
		t.Fatal("the QR code is drawn inline")
	}
	factor, secret := factorRe.FindStringSubmatch(page)[1], strings.ReplaceAll(secretRe.FindStringSubmatch(page)[1], " ", "")
	resp = h.post(c, "/mfa/setup/app", url.Values{"csrf": {csrfOf(t, page)}, "factor": {factor}, "code": {identity.TOTPCode(secret, time.Now())}})
	recovery := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || strings.Count(recovery, "<li><code>") != 10 {
		t.Fatalf("recovery codes after the first factor: %d %s", resp.StatusCode, recovery)
	}
	resp = h.post(c, "/mfa/setup/done", url.Values{"csrf": {csrfOf(t, recovery)}, "method": {"otp"}})
	if code := h.followToClient(c, resp); code == "" {
		t.Fatal("no code after setting up")
	}
	if amr := latestAMR(t, h); !hasAll(amr, "pwd", "otp", "mfa") {
		t.Fatalf("amr: %v", amr)
	}
}

func TestAResetLinkDoesNotSkipTheSecondFactor(t *testing.T) {
	h := newHarness(t)
	h.addTOTP("ana@example.com")
	ctx := context.Background()
	if err := h.ids().RequestPasswordReset(ctx, "ana@example.com"); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := h.store.Pool.QueryRow(ctx, `SELECT body_text FROM email_outbox WHERE to_addr = 'ana@example.com' AND subject LIKE 'Reset%' ORDER BY created_at DESC LIMIT 1`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	link := regexp.MustCompile(`(http://\S+/reset\?\S+)`).FindString(body)
	c := browser()
	page := readBody(t, h.get(c, link))
	u, _ := url.Parse(link)
	resp := h.post(c, "/reset", url.Values{"csrf": {csrfOf(t, page)}, "r": {u.Query().Get("r")}, "t": {u.Query().Get("t")},
		"password": {"a new long password"}, "confirm": {"a new long password"}})
	if got := location(t, resp); got.Path != "/mfa" {
		t.Fatalf("a reset link is one factor; the app is still asked for, went to %s", got)
	}
}

func TestConfirmItsYouBeforeSensitiveChanges(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	c := browser()
	_, ch := pkce()
	h.signIn(c, ch)
	if _, err := h.store.Pool.Exec(ctx, `UPDATE sessions SET verified_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	// Setting up a factor from Account asks first.
	if got := location(t, h.get(c, h.url+"/mfa/setup?return_to=http://console.test/account")); got.Path != "/reauth" {
		t.Fatalf("an old session confirms first: %s", got)
	}

	page := readBody(t, h.get(c, h.url+"/reauth?return_to="+url.QueryEscape("http://console.test/account")))
	csrf := csrfOf(t, page)
	if bad := h.post(c, "/reauth", url.Values{"csrf": {csrf}, "password": {"not my password!"}, "return_to": {"http://console.test/account"}}); bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a wrong password: %d", bad.StatusCode)
	}
	resp := h.post(c, "/reauth", url.Values{"csrf": {csrf}, "password": {"correct horse battery"}, "return_to": {"http://console.test/account"}})
	if got := location(t, resp); got.String() != "http://console.test/account" {
		t.Fatalf("back where they were: %s", got)
	}
	var recent bool
	if err := h.store.Pool.QueryRow(ctx, `SELECT verified_at > now() - interval '1 minute' FROM sessions ORDER BY created_at DESC LIMIT 1`).Scan(&recent); err != nil || !recent {
		t.Fatalf("verified_at not refreshed: %v", err)
	}
	// Now the setup page opens, and somewhere else is never a return address.
	if resp := h.get(c, h.url+"/mfa/setup?return_to=https://evil.test/"); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup after confirming: %d", resp.StatusCode)
	}
	resp = h.post(c, "/reauth", url.Values{"csrf": {csrf}, "password": {"correct horse battery"}, "return_to": {"https://evil.test/steal"}})
	if got := location(t, resp); got.String() != "http://console.test" {
		t.Fatalf("an outside return address must be dropped: %s", got)
	}
}

func TestAddAPasskeyFromAccount(t *testing.T) {
	h := newHarness(t)
	c := browser()
	_, ch := pkce()
	h.signIn(c, ch)
	page := readBody(t, h.get(c, h.url+"/mfa/setup?return_to=http://console.test/account"))
	if !strings.Contains(page, `data-passkey="register"`) {
		t.Fatal("the setup page offers a passkey")
	}
	key := testkey.New(h.url)
	to := h.passkeyCeremony(c, csrfOf(t, page), key, map[string]any{"purpose": "register", "name": "Laptop", "returnTo": "http://console.test/account"})
	resp := h.get(c, h.url+to)
	recovery := readBody(t, resp)
	if strings.Count(recovery, "<li><code>") != 10 || !strings.Contains(recovery, "http://console.test/account") {
		t.Fatalf("first factor: recovery codes, then back to Account: %s", recovery)
	}
	// Reloading the page mints nothing new.
	if got := location(t, h.get(c, h.url+to)); got.String() != "http://console.test/account" {
		t.Fatalf("reload: %s", got)
	}
	factors, _ := h.ids().Factors(context.Background(), h.user("ana@example.com").ID)
	if len(factors) != 1 || factors[0].Name != "Laptop" {
		t.Fatalf("factors: %+v", factors)
	}
}

func TestSingleSignOnAsksAgainOnceAFactorExists(t *testing.T) {
	h := newHarness(t)
	c := browser()
	_, ch := pkce()
	h.signIn(c, ch) // a password-only session
	h.addTOTP("ana@example.com")
	_, challenge := pkce()
	login := location(t, h.authorize(c, challenge, nil))
	if resp := h.get(c, login.String()); resp.StatusCode != http.StatusOK {
		t.Fatalf("a password-only session must not sign in someone who now has a second factor: %d", resp.StatusCode)
	}
}

func TestTurnstileGuardsSignUp(t *testing.T) {
	var asked int
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		_ = r.ParseForm()
		fmt.Fprintf(w, `{"success": %v}`, r.PostFormValue("response") == "human" && r.PostFormValue("secret") == "sekret")
	}))
	defer fake.Close()
	old := authserver.TurnstileVerifyURL
	authserver.TurnstileVerifyURL = fake.URL
	defer func() { authserver.TurnstileVerifyURL = old }()

	h := newHarnessWith(t, func(c *config.Config) { c.TurnstileSiteKey, c.TurnstileSecretKey = "site", "sekret" })
	c := browser()
	resp := h.get(c, h.url+"/signup")
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "https://challenges.cloudflare.com") {
		t.Fatalf("the widget's origin is allowed on this page only: %s", csp)
	}
	page := readBody(t, resp)
	if !strings.Contains(page, `data-sitekey="site"`) {
		t.Fatal("no widget")
	}
	if csp := h.get(c, h.url+"/login").Header.Get("Content-Security-Policy"); strings.Contains(csp, "cloudflare") {
		t.Fatalf("the login page loads nothing from elsewhere: %s", csp)
	}
	csrf := csrfOf(t, page)
	if bot := h.post(c, "/signup", url.Values{"csrf": {csrf}, "email": {"bot@example.com"}, "cf-turnstile-response": {"robot"}}); bot.StatusCode != http.StatusBadRequest {
		t.Fatalf("a failed check: %d", bot.StatusCode)
	}
	if ok := h.post(c, "/signup", url.Values{"csrf": {csrf}, "email": {"pat@example.com"}, "cf-turnstile-response": {"human"}}); ok.StatusCode != http.StatusOK {
		t.Fatalf("a passed check: %d", ok.StatusCode)
	}
	if asked != 2 {
		t.Fatalf("each answer is checked with Cloudflare: %d", asked)
	}
}

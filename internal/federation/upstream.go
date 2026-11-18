package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
)

// stateTTL is how long someone has to finish at the provider.
const stateTTL = 10 * time.Minute

var (
	ErrStateExpired = errs.New("this sign-in took too long or was already used; start again")
	ErrNoEmail      = errs.New("the provider didn't share a verified email address, so Hayat can't tell who you are")
)

// Endpoints are where a provider signs people in and says who they are.
type Endpoints struct {
	Auth, Token, UserInfo string
	// Emails is GitHub's list of the account's addresses with their verified flags.
	Emails string
}

var builtinEndpoints = map[string]Endpoints{
	KindGoogle: {
		Auth:     "https://accounts.google.com/o/oauth2/v2/auth",
		Token:    "https://oauth2.googleapis.com/token",
		UserInfo: "https://openidconnect.googleapis.com/v1/userinfo",
	},
	KindGitHub: {
		Auth:     "https://github.com/login/oauth/authorize",
		Token:    "https://github.com/login/oauth/access_token",
		UserInfo: "https://api.github.com/user",
		Emails:   "https://api.github.com/user/emails",
	},
}

var scopes = map[string][]string{
	KindGoogle: {"openid", "email", "profile"},
	KindGitHub: {"read:user", "user:email"},
	KindOIDC:   {"openid", "email", "profile"},
}

// SetEndpoints points a built-in provider somewhere else: a test server, or
// GitHub Enterprise.
func (f *Service) SetEndpoints(kind string, e Endpoints) { f.endpoints[kind] = e }

// SetLookupTXT replaces the DNS lookup domain verification uses (tests).
func (f *Service) SetLookupTXT(fn func(ctx context.Context, name string) ([]string, error)) {
	f.lookupTXT = fn
}

// ---------- OpenID Connect discovery, cached per issuer ----------

type discoveryCache struct {
	mu   sync.Mutex
	docs map[string]cachedDiscovery
}

type cachedDiscovery struct {
	e   Endpoints
	exp time.Time
}

func (c *discoveryCache) forget(issuer string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.docs, issuer)
}

func (f *Service) endpointsFor(ctx context.Context, p Provider) (Endpoints, error) {
	if p.Kind != KindOIDC {
		return f.endpoints[p.Kind], nil
	}
	c := &f.discovery
	c.mu.Lock()
	if d, ok := c.docs[p.Issuer]; ok && time.Now().Before(d.exp) {
		c.mu.Unlock()
		return d.e, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return Endpoints{}, err
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return Endpoints{}, fmt.Errorf("reach %s: %w", p.Issuer, err)
	}
	defer resp.Body.Close()
	var doc struct {
		Issuer   string `json:"issuer"`
		Auth     string `json:"authorization_endpoint"`
		Token    string `json:"token_endpoint"`
		UserInfo string `json:"userinfo_endpoint"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc) != nil {
		return Endpoints{}, errs.Newf("%s doesn't publish OpenID Connect discovery; check the issuer address", p.Issuer)
	}
	if strings.TrimRight(doc.Issuer, "/") != p.Issuer || doc.Auth == "" || doc.Token == "" || doc.UserInfo == "" {
		return Endpoints{}, errs.Newf("%s's discovery names another issuer or lacks an endpoint Hayat needs", p.Issuer)
	}
	e := Endpoints{Auth: doc.Auth, Token: doc.Token, UserInfo: doc.UserInfo}
	c.mu.Lock()
	if c.docs == nil {
		c.docs = map[string]cachedDiscovery{}
	}
	c.docs[p.Issuer] = cachedDiscovery{e: e, exp: time.Now().Add(time.Hour)}
	c.mu.Unlock()
	return e, nil
}

func (f *Service) oauthConfig(ctx context.Context, p Provider) (*oauth2.Config, Endpoints, error) {
	e, err := f.endpointsFor(ctx, p)
	if err != nil {
		return nil, Endpoints{}, err
	}
	secret, err := f.box.Open(p.secret)
	if err != nil {
		return nil, Endpoints{}, fmt.Errorf("open %s's client secret: %w", p.Name, err)
	}
	return &oauth2.Config{
		ClientID: p.ClientID, ClientSecret: string(secret), RedirectURL: f.CallbackURL(), Scopes: scopes[p.Kind],
		Endpoint: oauth2.Endpoint{AuthURL: e.Auth, TokenURL: e.Token, AuthStyle: oauth2.AuthStyleInParams},
	}, e, nil
}

// Start is what Begin hands back: where to send the browser, and the state
// to bind to it (a cookie), so the callback can't be replayed in another browser.
type Start struct {
	URL, State string
}

// Begin starts a trip to the provider. authRequestID is the sign-in it will
// finish, if any; linkUser links the account to someone already signed in.
func (f *Service) Begin(ctx context.Context, p Provider, authRequestID string, linkUser *uuid.UUID, loginHint string) (Start, error) {
	if !p.Enabled {
		return Start{}, ErrProviderOff
	}
	cfg, _, err := f.oauthConfig(ctx, p)
	if err != nil {
		return Start{}, err
	}
	state, verifier := crypto.NewToken(), oauth2.GenerateVerifier()
	if _, err := f.store.Pool.Exec(ctx, `INSERT INTO sso_states (state_hash, provider_id, auth_request_id, link_user_id, verifier_sealed, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, crypto.HashToken(state), p.ID, authRequestID, linkUser, f.box.Seal([]byte(verifier)), time.Now().Add(stateTTL)); err != nil {
		return Start{}, err
	}
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if loginHint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", loginHint))
	}
	if p.Kind == KindGoogle {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	return Start{URL: cfg.AuthCodeURL(state, opts...), State: state}, nil
}

// Profile is who the provider says signed in.
type Profile struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// Return is a finished trip: the provider, who it vouched for, and what the
// trip was for.
type Return struct {
	Provider      Provider
	Profile       Profile
	AuthRequestID string
	LinkUser      *uuid.UUID
}

// Complete finishes a trip: the state is used up, the code exchanged (with
// the PKCE verifier), and the person read from the provider's own API.
func (f *Service) Complete(ctx context.Context, state, code string) (Return, error) {
	var r Return
	var providerID uuid.UUID
	var sealed []byte
	var exp time.Time
	err := f.store.Pool.QueryRow(ctx, `DELETE FROM sso_states WHERE state_hash = $1
		RETURNING provider_id, auth_request_id, link_user_id, verifier_sealed, expires_at`, crypto.HashToken(state)).
		Scan(&providerID, &r.AuthRequestID, &r.LinkUser, &sealed, &exp)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && time.Now().After(exp)) {
		return Return{}, ErrStateExpired
	}
	if err != nil {
		return Return{}, err
	}
	verifier, err := f.box.Open(sealed)
	if err != nil {
		return Return{}, err
	}
	if r.Provider, err = f.Provider(ctx, providerID); err != nil {
		return Return{}, err
	}
	if !r.Provider.Enabled {
		return Return{}, ErrProviderOff
	}
	cfg, e, err := f.oauthConfig(ctx, r.Provider)
	if err != nil {
		return Return{}, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, f.http)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(string(verifier)))
	if err != nil {
		f.log.Warn("sign-in provider refused the code", "provider", r.Provider.Name, "err", err)
		return Return{}, errs.Newf("%s didn't accept the sign-in; start again", r.Provider.Name)
	}
	if r.Profile, err = f.profile(ctx, r.Provider, e, tok.AccessToken); err != nil {
		return Return{}, err
	}
	return r, nil
}

func (f *Service) getJSON(ctx context.Context, url, token string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// flexBool reads email_verified, which some providers send as "true".
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	*b = flexBool(s == "true")
	return nil
}

func (f *Service) profile(ctx context.Context, p Provider, e Endpoints, token string) (Profile, error) {
	if p.Kind == KindGitHub {
		var u struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := f.getJSON(ctx, e.UserInfo, token, &u); err != nil || u.ID == 0 {
			return Profile{}, fmt.Errorf("read the GitHub account: %w", err)
		}
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := f.getJSON(ctx, e.Emails, token, &emails); err != nil {
			return Profile{}, fmt.Errorf("read the GitHub account's emails: %w", err)
		}
		out := Profile{Subject: strconv.FormatInt(u.ID, 10), Name: u.Name, Picture: u.AvatarURL}
		if out.Name == "" {
			out.Name = u.Login
		}
		// The primary address if GitHub verified it; never an unverified one.
		for _, m := range emails {
			if m.Verified && (m.Primary || out.Email == "") {
				out.Email, out.EmailVerified = m.Email, true
			}
		}
		return out, nil
	}
	var u struct {
		Sub           string   `json:"sub"`
		Email         string   `json:"email"`
		EmailVerified flexBool `json:"email_verified"`
		Name          string   `json:"name"`
		Picture       string   `json:"picture"`
	}
	if err := f.getJSON(ctx, e.UserInfo, token, &u); err != nil || u.Sub == "" {
		return Profile{}, fmt.Errorf("read the %s account: %w", p.Name, err)
	}
	return Profile{Subject: u.Sub, Email: u.Email, EmailVerified: bool(u.EmailVerified), Name: u.Name, Picture: u.Picture}, nil
}

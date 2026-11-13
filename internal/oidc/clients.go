package oidc

import (
	"context"
	"crypto/subtle"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/store"
)

// ConsoleClientID is the built-in client the console signs in with.
const ConsoleClientID = "hayat-console"

// PlatformService is the service the console client, and any client registered
// before clients belonged to services, belongs to. It is Hayat itself.
const PlatformService = "hayat"

const (
	KindPublic       = "public"
	KindConfidential = "confidential"
)

var allowedGrants = []string{
	string(oidc.GrantTypeCode),
	string(oidc.GrantTypeRefreshToken),
	string(oidc.GrantTypeClientCredentials),
}

var ErrBuiltin = errs.New("the console client is managed by Hayat itself")

// Client is one registered application. It implements op.Client. Every client
// belongs to a service: its tokens name that service in their audience.
type Client struct {
	ID                  string
	ServiceID           string
	Name                string
	Kind                string
	Redirects           []string
	PostLogoutRedirects []string
	Grants              []string
	Builtin             bool
	DisabledAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	secretHash          []byte
	issuer              string
	dev                 bool
}

func (c *Client) GetID() string                    { return c.ID }
func (c *Client) RedirectURIs() []string           { return c.Redirects }
func (c *Client) PostLogoutRedirectURIs() []string { return c.PostLogoutRedirects }
func (c *Client) LoginURL(id string) string {
	return c.issuer + "/login?authRequestID=" + url.QueryEscape(id)
}
func (c *Client) ApplicationType() op.ApplicationType {
	if c.Kind == KindPublic {
		return op.ApplicationTypeUserAgent
	}
	return op.ApplicationTypeWeb
}
func (c *Client) AuthMethod() oidc.AuthMethod {
	if c.Kind == KindPublic {
		return oidc.AuthMethodNone
	}
	return oidc.AuthMethodBasic
}
func (c *Client) ResponseTypes() []oidc.ResponseType {
	return []oidc.ResponseType{oidc.ResponseTypeCode}
}
func (c *Client) GrantTypes() []oidc.GrantType {
	out := make([]oidc.GrantType, len(c.Grants))
	for i, g := range c.Grants {
		out[i] = oidc.GrantType(g)
	}
	return out
}
func (c *Client) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeJWT }
func (c *Client) IDTokenLifetime() time.Duration      { return time.Hour }
func (c *Client) DevMode() bool                       { return c.dev }
func (c *Client) RestrictAdditionalIdTokenScopes() func([]string) []string {
	return func(s []string) []string { return s }
}
func (c *Client) RestrictAdditionalAccessTokenScopes() func([]string) []string {
	return func(s []string) []string { return s }
}
func (c *Client) IsScopeAllowed(string) bool           { return false }
func (c *Client) IDTokenUserinfoClaimsAssertion() bool { return true }
func (c *Client) ClockSkew() time.Duration             { return 0 }

func (c *Client) checkSecret(secret string) bool {
	if c.Kind != KindConfidential || len(c.secretHash) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(crypto.HashToken(secret), c.secretHash) == 1
}

// Clients owns oauth_clients.
type Clients struct {
	store  *store.Store
	issuer string
	dev    bool
}

func NewClients(s *store.Store, issuer string, dev bool) *Clients {
	return &Clients{store: s, issuer: strings.TrimRight(issuer, "/"), dev: dev}
}

const clientColumns = `id, service_id, name, kind, secret_hash, redirect_uris, post_logout_redirect_uris, grant_types, builtin, disabled_at, created_at, updated_at`

func (cs *Clients) scan(row pgx.Row) (*Client, error) {
	c := &Client{issuer: cs.issuer, dev: cs.dev}
	err := row.Scan(&c.ID, &c.ServiceID, &c.Name, &c.Kind, &c.secretHash, &c.Redirects, &c.PostLogoutRedirects, &c.Grants, &c.Builtin,
		&c.DisabledAt, &c.CreatedAt, &c.UpdatedAt)
	return c, store.NotFound(err)
}

func (cs *Clients) Get(ctx context.Context, id string) (*Client, error) {
	return cs.scan(cs.store.Pool.QueryRow(ctx, `SELECT `+clientColumns+` FROM oauth_clients WHERE id = $1`, id))
}

// ForService lists the clients of one service, the built-in console first.
func (cs *Clients) ForService(ctx context.Context, serviceID string) ([]*Client, error) {
	rows, err := cs.store.Pool.Query(ctx, `SELECT `+clientColumns+` FROM oauth_clients WHERE service_id = $1
		ORDER BY builtin DESC, created_at`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Client
	for rows.Next() {
		c, err := cs.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// EnsureConsole registers the console client, or updates its addresses when
// HAYAT_CONSOLE_URL changed.
func (cs *Clients) EnsureConsole(ctx context.Context, redirectURI, consoleURL string) error {
	_, err := cs.store.Pool.Exec(ctx, `INSERT INTO oauth_clients (id, service_id, name, kind, redirect_uris, post_logout_redirect_uris, grant_types, builtin)
		VALUES ($1, $2, 'Hayat console', 'public', $3, $4, '{authorization_code,refresh_token}', true)
		ON CONFLICT (id) DO UPDATE SET redirect_uris = EXCLUDED.redirect_uris,
			post_logout_redirect_uris = EXCLUDED.post_logout_redirect_uris, updated_at = now()`,
		ConsoleClientID, PlatformService, []string{redirectURI}, []string{consoleURL, consoleURL + "/"})
	return err
}

type ClientInput struct {
	// ServiceID is the service the client belongs to. Required on Create;
	// a client never moves to another service.
	ServiceID           string
	Name                string
	Kind                string
	Redirects           []string
	PostLogoutRedirects []string
	Grants              []string
}

func (cs *Clients) validate(in *ClientInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Redirects, in.PostLogoutRedirects = nonNil(in.Redirects), nonNil(in.PostLogoutRedirects)
	if in.Name == "" || len(in.Name) > 100 {
		return errs.New("give the client a name of up to 100 characters")
	}
	if in.Kind != KindPublic && in.Kind != KindConfidential {
		return errs.New("a client is public (a browser or mobile app) or confidential (a server)")
	}
	if len(in.Grants) == 0 {
		in.Grants = []string{string(oidc.GrantTypeCode), string(oidc.GrantTypeRefreshToken)}
	}
	for _, g := range in.Grants {
		if !slices.Contains(allowedGrants, g) {
			return errs.Newf("unsupported grant type %q", g)
		}
	}
	if in.Kind == KindPublic && slices.Contains(in.Grants, string(oidc.GrantTypeClientCredentials)) {
		return errs.New("a public client can't use client credentials: it has no secret to keep")
	}
	needsRedirect := slices.Contains(in.Grants, string(oidc.GrantTypeCode))
	if needsRedirect && len(in.Redirects) == 0 {
		return errs.New("add at least one redirect URI for the sign-in flow")
	}
	for _, list := range [][]string{in.Redirects, in.PostLogoutRedirects} {
		for _, raw := range list {
			if err := cs.checkRedirect(raw); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkRedirect accepts https URIs, and http only for localhost (or anything in
// dev mode). Fragments are never allowed: the code would leak into them.
func (cs *Clients) checkRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" {
		return errs.Newf("%q is not a valid redirect URI", raw)
	}
	switch {
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && (cs.dev || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"):
		return nil
	}
	return errs.Newf("%q must use https (http is allowed only for localhost)", raw)
}

// Create registers a client. For a confidential client it returns the secret,
// which is shown once and stored only as a hash.
func (cs *Clients) Create(ctx context.Context, in ClientInput, actor string) (*Client, string, error) {
	if strings.TrimSpace(in.ServiceID) == "" {
		return nil, "", errs.New("a client belongs to a service; name the service")
	}
	if err := cs.validate(&in); err != nil {
		return nil, "", err
	}
	id := "c_" + strings.ToLower(strings.ReplaceAll(crypto.NewToken()[:18], "_", "x"))
	id = strings.ReplaceAll(id, "-", "x")
	secret := ""
	var hash []byte
	if in.Kind == KindConfidential {
		secret = crypto.NewToken()
		hash = crypto.HashToken(secret)
	}
	var c *Client
	err := cs.store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		c, err = cs.scan(tx.QueryRow(ctx, `INSERT INTO oauth_clients (id, service_id, name, kind, secret_hash, redirect_uris, post_logout_redirect_uris, grant_types)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+clientColumns,
			id, in.ServiceID, in.Name, in.Kind, hash, nonNil(in.Redirects), nonNil(in.PostLogoutRedirects), in.Grants))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, cs.store.Region, actor, "client.created", "client:"+id,
			map[string]any{"name": in.Name, "kind": in.Kind, "service": in.ServiceID})
	})
	return c, secret, err
}

// Update changes a client's name, addresses and grants. Its kind is fixed.
func (cs *Clients) Update(ctx context.Context, id string, in ClientInput, actor string) (*Client, error) {
	cur, err := cs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.Builtin {
		return nil, ErrBuiltin
	}
	in.Kind = cur.Kind
	if err := cs.validate(&in); err != nil {
		return nil, err
	}
	var c *Client
	err = cs.store.Tx(ctx, func(tx pgx.Tx) error {
		c, err = cs.scan(tx.QueryRow(ctx, `UPDATE oauth_clients SET name = $2, redirect_uris = $3, post_logout_redirect_uris = $4,
			grant_types = $5, updated_at = now() WHERE id = $1 RETURNING `+clientColumns,
			id, in.Name, nonNil(in.Redirects), nonNil(in.PostLogoutRedirects), in.Grants))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, cs.store.Region, actor, "client.updated", "client:"+id, nil)
	})
	return c, err
}

// RotateSecret replaces a confidential client's secret. The old one stops
// working at once.
func (cs *Clients) RotateSecret(ctx context.Context, id, actor string) (string, error) {
	cur, err := cs.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if cur.Kind != KindConfidential {
		return "", errs.New("only a confidential client has a secret")
	}
	secret := crypto.NewToken()
	err = cs.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE oauth_clients SET secret_hash = $2, updated_at = now() WHERE id = $1`, id, crypto.HashToken(secret)); err != nil {
			return err
		}
		return audit.Write(ctx, tx, cs.store.Region, actor, "client.secret_rotated", "client:"+id, nil)
	})
	return secret, err
}

// SetDisabled turns a client off or on. A disabled client can't start a
// sign-in, exchange a code or refresh a token.
func (cs *Clients) SetDisabled(ctx context.Context, id string, disabled bool, actor string) (*Client, error) {
	cur, err := cs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.Builtin {
		return nil, ErrBuiltin
	}
	var at any
	if disabled {
		at = time.Now()
	}
	var c *Client
	err = cs.store.Tx(ctx, func(tx pgx.Tx) error {
		c, err = cs.scan(tx.QueryRow(ctx, `UPDATE oauth_clients SET disabled_at = $2, updated_at = now() WHERE id = $1 RETURNING `+clientColumns, id, at))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, cs.store.Region, actor, "client.disabled_changed", "client:"+id, map[string]any{"disabled": disabled})
	})
	return c, err
}

// Delete removes a client outright. Used to undo a half-finished service
// registration; admins turn clients off instead.
func (cs *Clients) Delete(ctx context.Context, id string) error {
	_, err := cs.store.Pool.Exec(ctx, `DELETE FROM oauth_clients WHERE id = $1 AND NOT builtin`, id)
	return err
}

func (cs *Clients) Count(ctx context.Context) (int, error) {
	var n int
	err := cs.store.Pool.QueryRow(ctx, `SELECT count(*) FROM oauth_clients`).Scan(&n)
	return n, err
}

// CountByService counts each service's clients, its API credential included.
func (cs *Clients) CountByService(ctx context.Context) (map[string]int, error) {
	rows, err := cs.store.Pool.Query(ctx, `SELECT service_id, count(*) FROM oauth_clients GROUP BY service_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var svc string
		var n int
		if err := rows.Scan(&svc, &n); err != nil {
			return nil, err
		}
		out[svc] = n
	}
	return out, rows.Err()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	out := make([]string, 0, len(s))
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

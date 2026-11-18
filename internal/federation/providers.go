// Package federation signs people in through another account: Google and
// GitHub for the whole instance, and each tenant's own company identity
// provider (OpenID Connect) for the email domains it has proved it owns.
//
// It owns identity_providers, user_identities, identity_refs, tenant_domains
// and sso_states. Hayat stays the identity provider for every service: an
// outside sign-in ends in an ordinary Hayat session, so services never learn
// which provider was used beyond `amr: ["fed"]`.
package federation

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
	"github.com/aidapedia/hayat/internal/tenancy"
)

const (
	KindGoogle = "google"
	KindGitHub = "github"
	KindOIDC   = "oidc"
)

var (
	ErrNoSecret       = errs.New("paste the client secret the provider gave you")
	ErrNoClientID     = errs.New("paste the client ID the provider gave you")
	ErrProviderOff    = errs.New("this way of signing in is turned off; use another one")
	ErrWrongKind      = errs.New("the instance offers Google, GitHub and one company OpenID Connect provider")
	ErrIssuer         = errs.New("the issuer must be the provider's https:// address, like https://acme.okta.com")
	ErrProviderInUse  = errs.New("a domain set to company sign-in only uses this provider; switch that domain to offer first")
	errProviderTenant = errs.New("that provider isn't one of this tenant's")
)

// Provider is a way to sign in with another account. Its secret never leaves
// this package: HasSecret says whether one is set.
type Provider struct {
	ID        uuid.UUID
	TenantID  *uuid.UUID
	Kind      string
	Name      string
	Issuer    string
	ClientID  string
	Enabled   bool
	JIT       bool
	HasSecret bool
	CreatedAt time.Time
	UpdatedAt time.Time

	secret []byte // sealed
}

// ProviderInput is what an admin sets. An empty ClientSecret keeps the one stored.
type ProviderInput struct {
	Name, Issuer, ClientID, ClientSecret string
	Enabled, JIT                         bool
}

type Service struct {
	store   *store.Store
	box     *crypto.Box
	ids     *identity.Service
	tenants *tenancy.Service
	issuer  string
	dev     bool
	log     *slog.Logger

	http      *http.Client
	endpoints map[string]Endpoints
	lookupTXT func(ctx context.Context, name string) ([]string, error)
	discovery discoveryCache
}

// New builds the service. issuer is the auth role's public URL: providers
// send people back to <issuer>/sso/callback.
func New(s *store.Store, box *crypto.Box, ids *identity.Service, tenants *tenancy.Service, issuer string, dev bool, log *slog.Logger) *Service {
	ep := map[string]Endpoints{}
	for k, v := range builtinEndpoints {
		ep[k] = v
	}
	return &Service{store: s, box: box, ids: ids, tenants: tenants, issuer: strings.TrimRight(issuer, "/"), dev: dev, log: log,
		http: &http.Client{Timeout: 15 * time.Second}, endpoints: ep, lookupTXT: net.DefaultResolver.LookupTXT}
}

// CallbackURL is the redirect URI every provider is registered with.
func (f *Service) CallbackURL() string { return f.issuer + "/sso/callback" }

const providerColumns = `id, tenant_id, kind, name, issuer, client_id, enabled, jit, secret_sealed, created_at, updated_at`

func scanProvider(row pgx.Row) (Provider, error) {
	var p Provider
	err := row.Scan(&p.ID, &p.TenantID, &p.Kind, &p.Name, &p.Issuer, &p.ClientID, &p.Enabled, &p.JIT, &p.secret, &p.CreatedAt, &p.UpdatedAt)
	p.HasSecret = len(p.secret) > 0
	return p, store.NotFound(err)
}

func (f *Service) listProviders(ctx context.Context, where string, args ...any) ([]Provider, error) {
	rows, err := f.store.Pool.Query(ctx, `SELECT `+providerColumns+` FROM identity_providers WHERE `+where+` ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InstanceProviders are the instance's own: Google and GitHub, when set up.
func (f *Service) InstanceProviders(ctx context.Context) ([]Provider, error) {
	return f.listProviders(ctx, `tenant_id IS NULL`)
}

// TenantProviders are a tenant's company identity providers.
func (f *Service) TenantProviders(ctx context.Context, tenantID uuid.UUID) ([]Provider, error) {
	return f.listProviders(ctx, `tenant_id = $1`, tenantID)
}

func (f *Service) Provider(ctx context.Context, id uuid.UUID) (Provider, error) {
	return scanProvider(f.store.Pool.QueryRow(ctx, `SELECT `+providerColumns+` FROM identity_providers WHERE id = $1`, id))
}

func (f *Service) checkIssuer(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrIssuer
	}
	if u.Scheme != "https" && !(f.dev && u.Scheme == "http") { // http only in development
		return "", ErrIssuer
	}
	return raw, nil
}

func (f *Service) seal(secret string) []byte {
	return f.box.Seal([]byte(strings.TrimSpace(secret)))
}

// SaveInstanceProvider sets up a way in for everyone on the instance, or
// changes it: Google, GitHub, or one company OpenID Connect provider (for an
// instance that is one organisation: its Okta, Entra ID or Keycloak, trusted
// for any address it vouches for). The first save needs the client secret.
func (f *Service) SaveInstanceProvider(ctx context.Context, kind string, in ProviderInput, actor string) (Provider, error) {
	if kind != KindGoogle && kind != KindGitHub && kind != KindOIDC {
		return Provider{}, ErrWrongKind
	}
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		return Provider{}, ErrNoClientID
	}
	name := map[string]string{KindGoogle: "Google", KindGitHub: "GitHub"}[kind]
	issuer := ""
	if kind == KindOIDC {
		var err error
		if issuer, err = f.checkIssuer(in.Issuer); err != nil {
			return Provider{}, err
		}
		if name = strings.Join(strings.Fields(in.Name), " "); name == "" || len([]rune(name)) > 60 {
			return Provider{}, errs.New("name the provider in up to 60 characters, like Company SSO")
		}
	}
	var id uuid.UUID
	err := f.store.Tx(ctx, func(tx pgx.Tx) error {
		var hasSecret bool
		err := tx.QueryRow(ctx, `SELECT id, octet_length(secret_sealed) > 0 FROM identity_providers WHERE tenant_id IS NULL AND kind = $1 FOR UPDATE`, kind).Scan(&id, &hasSecret)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if strings.TrimSpace(in.ClientSecret) == "" {
				return ErrNoSecret
			}
			id = store.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO identity_providers (id, kind, name, issuer, client_id, secret_sealed, enabled, created_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, kind, name, issuer, clientID, f.seal(in.ClientSecret), in.Enabled, actor); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if _, err := tx.Exec(ctx, `UPDATE identity_providers SET name = $2, issuer = $3, client_id = $4, enabled = $5, updated_at = now() WHERE id = $1`,
				id, name, issuer, clientID, in.Enabled); err != nil {
				return err
			}
			if strings.TrimSpace(in.ClientSecret) != "" {
				if _, err := tx.Exec(ctx, `UPDATE identity_providers SET secret_sealed = $2 WHERE id = $1`, id, f.seal(in.ClientSecret)); err != nil {
					return err
				}
			}
		}
		return audit.Write(ctx, tx, f.store.Region, actor, "sign_in.provider_saved", "provider:"+kind, map[string]any{"enabled": in.Enabled})
	})
	if err != nil {
		return Provider{}, err
	}
	f.discovery.forget(issuer)
	return f.Provider(ctx, id)
}

// SaveTenantProvider connects a tenant's company identity provider, or
// changes it (id set). The first save needs the client secret.
func (f *Service) SaveTenantProvider(ctx context.Context, tenantID uuid.UUID, id *uuid.UUID, in ProviderInput, actor string) (Provider, error) {
	issuer, err := f.checkIssuer(in.Issuer)
	if err != nil {
		return Provider{}, err
	}
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		return Provider{}, ErrNoClientID
	}
	name := strings.Join(strings.Fields(in.Name), " ")
	if name == "" || len([]rune(name)) > 60 {
		return Provider{}, errs.New("name the provider in up to 60 characters, like Acme SSO")
	}
	var out uuid.UUID
	err = f.store.Tx(ctx, func(tx pgx.Tx) error {
		if id == nil {
			if strings.TrimSpace(in.ClientSecret) == "" {
				return ErrNoSecret
			}
			out = store.NewID()
			if _, err := tx.Exec(ctx, `INSERT INTO identity_providers (id, tenant_id, kind, name, issuer, client_id, secret_sealed, enabled, jit, created_by)
				VALUES ($1, $2, 'oidc', $3, $4, $5, $6, $7, $8, $9)`, out, tenantID, name, issuer, clientID, f.seal(in.ClientSecret), in.Enabled, in.JIT, actor); err != nil {
				return err
			}
		} else {
			out = *id
			tag, err := tx.Exec(ctx, `UPDATE identity_providers SET name = $3, issuer = $4, client_id = $5, enabled = $6, jit = $7, updated_at = now()
				WHERE id = $1 AND tenant_id = $2`, out, tenantID, name, issuer, clientID, in.Enabled, in.JIT)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return errProviderTenant
			}
			if strings.TrimSpace(in.ClientSecret) != "" {
				if _, err := tx.Exec(ctx, `UPDATE identity_providers SET secret_sealed = $2 WHERE id = $1`, out, f.seal(in.ClientSecret)); err != nil {
					return err
				}
			}
			if !in.Enabled {
				// A domain can't force a provider nobody can use.
				var forced bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_domains WHERE provider_id = $1 AND policy = 'enforce')`, out).Scan(&forced); err != nil {
					return err
				}
				if forced {
					return ErrProviderInUse
				}
			}
		}
		return audit.Write(ctx, tx, f.store.Region, actor, "sign_in.tenant_provider_saved", "tenant:"+tenantID.String(),
			map[string]any{"provider": out, "issuer": issuer, "enabled": in.Enabled, "jit": in.JIT})
	})
	if err != nil {
		return Provider{}, err
	}
	f.discovery.forget(issuer)
	return f.Provider(ctx, out)
}

// DeleteTenantProvider disconnects a tenant's provider. Links to it stay, so
// connecting the same issuer again doesn't bring them back — people link anew.
// Refused while a domain forces it.
func (f *Service) DeleteTenantProvider(ctx context.Context, tenantID, id uuid.UUID, actor string) error {
	return f.store.Tx(ctx, func(tx pgx.Tx) error {
		var forced bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_domains WHERE provider_id = $1 AND policy = 'enforce')`, id).Scan(&forced); err != nil {
			return err
		}
		if forced {
			return ErrProviderInUse
		}
		if _, err := tx.Exec(ctx, `UPDATE tenant_domains SET provider_id = NULL WHERE provider_id = $1`, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM identity_providers WHERE id = $1 AND tenant_id = $2`, id, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errProviderTenant
		}
		return audit.Write(ctx, tx, f.store.Region, actor, "sign_in.tenant_provider_deleted", "tenant:"+tenantID.String(), map[string]any{"provider": id})
	})
}

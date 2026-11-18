package federation

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/store"
)

const (
	PolicyOffer   = "offer"
	PolicyEnforce = "enforce"
)

var (
	ErrDomainInvalid  = errs.New("enter a domain like acme.com, without @ or https://")
	ErrDomainClaimed  = errs.New("another tenant has already verified this domain")
	ErrNotVerified    = errs.New("the TXT record isn't there yet. DNS can take a few minutes to update; try again shortly")
	ErrEnforceNeeds   = errs.New("company sign-in only needs a verified domain and an enabled provider")
	errDomainMissing  = errs.New("this tenant hasn't added that domain")
	domainRe          = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	freeMailProviders = []string{"gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com", "yahoo.com", "icloud.com",
		"me.com", "proton.me", "protonmail.com", "aol.com", "gmx.com", "yandex.com", "mail.com"}
)

// Domain is an email domain a tenant claims. Until it is verified it does
// nothing: no routing, no enforcement, no account creation.
type Domain struct {
	Domain      string
	TenantID    uuid.UUID
	VerifyToken string
	VerifiedAt  *time.Time
	ProviderID  *uuid.UUID
	Policy      string
	CreatedAt   time.Time
}

func (d Domain) Verified() bool { return d.VerifiedAt != nil }

// TXTName and TXTValue are the record that proves the tenant controls the domain.
func (d Domain) TXTName() string  { return "_hayat-challenge." + d.Domain }
func (d Domain) TXTValue() string { return "hayat-verify=" + d.VerifyToken }

// NormalizeDomain lower-cases and checks a domain someone typed.
func NormalizeDomain(raw string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(raw))
	d = strings.TrimPrefix(strings.TrimSuffix(d, "."), "@")
	if len(d) > 253 || !domainRe.MatchString(d) {
		return "", ErrDomainInvalid
	}
	if slices.Contains(freeMailProviders, d) {
		return "", errs.Newf("%s is shared by everyone who uses it; add your company's own domain", d)
	}
	return d, nil
}

// DomainOf is the part after @, lower-cased.
func DomainOf(email string) string {
	_, d, ok := strings.Cut(strings.TrimSpace(email), "@")
	if !ok {
		return ""
	}
	return strings.ToLower(d)
}

const domainColumns = `domain, tenant_id, verify_token, verified_at, provider_id, policy, created_at`

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	err := row.Scan(&d.Domain, &d.TenantID, &d.VerifyToken, &d.VerifiedAt, &d.ProviderID, &d.Policy, &d.CreatedAt)
	return d, store.NotFound(err)
}

func (f *Service) TenantDomains(ctx context.Context, tenantID uuid.UUID) ([]Domain, error) {
	rows, err := f.store.Pool.Query(ctx, `SELECT `+domainColumns+` FROM tenant_domains WHERE tenant_id = $1 ORDER BY domain`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (f *Service) tenantDomain(ctx context.Context, db store.DB, tenantID uuid.UUID, domain string) (Domain, error) {
	d, err := scanDomain(db.QueryRow(ctx, `SELECT `+domainColumns+` FROM tenant_domains WHERE domain = $1`, domain))
	if errors.Is(err, store.ErrNotFound) || (err == nil && d.TenantID != tenantID) {
		return Domain{}, errDomainMissing
	}
	return d, err
}

// AddDomain claims a domain for the tenant and answers the TXT record that
// proves it. A claim another tenant never verified gives way: only the
// record decides who owns a domain.
func (f *Service) AddDomain(ctx context.Context, tenantID uuid.UUID, raw, actor string) (Domain, error) {
	domain, err := NormalizeDomain(raw)
	if err != nil {
		return Domain{}, err
	}
	region, err := f.tenantRegion(ctx, tenantID)
	if err != nil {
		return Domain{}, err
	}
	err = f.store.Tx(ctx, func(tx pgx.Tx) error {
		existing, err := scanDomain(tx.QueryRow(ctx, `SELECT `+domainColumns+` FROM tenant_domains WHERE domain = $1 FOR UPDATE`, domain))
		switch {
		case err == nil && existing.TenantID == tenantID:
			return nil
		case err == nil && existing.Verified():
			return ErrDomainClaimed
		case err == nil:
			if _, err := tx.Exec(ctx, `DELETE FROM tenant_domains WHERE domain = $1`, domain); err != nil {
				return err
			}
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tenant_domains (domain, tenant_id, region, verify_token, created_by) VALUES ($1, $2, $3, $4, $5)`,
			domain, tenantID, region, crypto.NewToken(), actor); err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "sign_in.domain_added", "tenant:"+tenantID.String(), map[string]any{"domain": domain})
	})
	if err != nil {
		return Domain{}, err
	}
	return f.tenantDomain(ctx, f.store.Pool, tenantID, domain)
}

func (f *Service) tenantRegion(ctx context.Context, tenantID uuid.UUID) (string, error) {
	t, err := f.tenants.Get(ctx, tenantID)
	if err != nil {
		return "", err
	}
	return t.Region, nil
}

// VerifyDomain looks the TXT record up and, when it is there, marks the domain verified.
func (f *Service) VerifyDomain(ctx context.Context, tenantID uuid.UUID, raw, actor string) (Domain, error) {
	domain, err := NormalizeDomain(raw)
	if err != nil {
		return Domain{}, err
	}
	d, err := f.tenantDomain(ctx, f.store.Pool, tenantID, domain)
	if err != nil {
		return Domain{}, err
	}
	if d.Verified() {
		return d, nil
	}
	records, err := f.lookupTXT(ctx, d.TXTName())
	if err != nil || !slices.Contains(records, d.TXTValue()) {
		return Domain{}, ErrNotVerified
	}
	err = f.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE tenant_domains SET verified_at = now() WHERE domain = $1 AND tenant_id = $2`, domain, tenantID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, f.store.Region, actor, "sign_in.domain_verified", "tenant:"+tenantID.String(), map[string]any{"domain": domain})
	})
	if err != nil {
		return Domain{}, err
	}
	return f.tenantDomain(ctx, f.store.Pool, tenantID, domain)
}

// SetDomainSignIn picks the provider people at the domain sign in through,
// and whether it is offered beside the password or the only way in.
func (f *Service) SetDomainSignIn(ctx context.Context, tenantID uuid.UUID, raw string, providerID *uuid.UUID, policy, actor string) (Domain, error) {
	domain, err := NormalizeDomain(raw)
	if err != nil {
		return Domain{}, err
	}
	if policy != PolicyOffer && policy != PolicyEnforce {
		return Domain{}, errs.New("choose offer or enforce")
	}
	err = f.store.Tx(ctx, func(tx pgx.Tx) error {
		d, err := f.tenantDomain(ctx, tx, tenantID, domain)
		if err != nil {
			return err
		}
		if providerID != nil {
			p, err := scanProvider(tx.QueryRow(ctx, `SELECT `+providerColumns+` FROM identity_providers WHERE id = $1`, *providerID))
			if err != nil || p.TenantID == nil || *p.TenantID != tenantID {
				return errProviderTenant
			}
			if policy == PolicyEnforce && !p.Enabled {
				return ErrEnforceNeeds
			}
		}
		if policy == PolicyEnforce && (!d.Verified() || providerID == nil) {
			return ErrEnforceNeeds
		}
		if _, err := tx.Exec(ctx, `UPDATE tenant_domains SET provider_id = $3, policy = $4 WHERE domain = $1 AND tenant_id = $2`,
			domain, tenantID, providerID, policy); err != nil {
			return err
		}
		return audit.Write(ctx, tx, f.store.Region, actor, "sign_in.domain_policy_set", "tenant:"+tenantID.String(),
			map[string]any{"domain": domain, "provider": providerID, "policy": policy})
	})
	if err != nil {
		return Domain{}, err
	}
	return f.tenantDomain(ctx, f.store.Pool, tenantID, domain)
}

func (f *Service) RemoveDomain(ctx context.Context, tenantID uuid.UUID, raw, actor string) error {
	domain, err := NormalizeDomain(raw)
	if err != nil {
		return err
	}
	return f.store.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM tenant_domains WHERE domain = $1 AND tenant_id = $2`, domain, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errDomainMissing
		}
		return audit.Write(ctx, tx, f.store.Region, actor, "sign_in.domain_removed", "tenant:"+tenantID.String(), map[string]any{"domain": domain})
	})
}

// CompanySignIn is the company sign-in for an email address: its verified
// domain and that domain's enabled provider. ok is false when there is none.
func (f *Service) CompanySignIn(ctx context.Context, email string) (Provider, Domain, bool, error) {
	domain := DomainOf(email)
	if domain == "" {
		return Provider{}, Domain{}, false, nil
	}
	d, err := scanDomain(f.store.Pool.QueryRow(ctx, `SELECT `+domainColumns+` FROM tenant_domains WHERE domain = $1 AND verified_at IS NOT NULL`, domain))
	if errors.Is(err, store.ErrNotFound) || (err == nil && d.ProviderID == nil) {
		return Provider{}, Domain{}, false, nil
	}
	if err != nil {
		return Provider{}, Domain{}, false, err
	}
	p, err := f.Provider(ctx, *d.ProviderID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !p.Enabled) {
		return Provider{}, Domain{}, false, nil
	}
	return p, d, err == nil, err
}

// Enforced says whether the address may only sign in through its company's
// provider. Instance admins never are: a broken provider must not lock out
// the people who can fix it.
func (f *Service) Enforced(ctx context.Context, email string) (Provider, bool, error) {
	p, d, ok, err := f.CompanySignIn(ctx, email)
	if err != nil || !ok || d.Policy != PolicyEnforce {
		return Provider{}, false, err
	}
	if u, err := f.ids.GetUserByEmail(ctx, email); err == nil && u.IsAdmin() {
		return Provider{}, false, nil
	}
	return p, true, nil
}

// HasCompanySignIn says whether any tenant has company sign-in ready, so the
// login page offers "Continue with company SSO" only when it can lead somewhere.
func (f *Service) HasCompanySignIn(ctx context.Context) (bool, error) {
	var ok bool
	err := f.store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tenant_domains d JOIN identity_providers p ON p.id = d.provider_id
		WHERE d.verified_at IS NOT NULL AND p.enabled)`).Scan(&ok)
	return ok, err
}

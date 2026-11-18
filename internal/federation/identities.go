package federation

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/errs"
	"github.com/aidapedia/hayat/internal/identity"
	"github.com/aidapedia/hayat/internal/store"
)

var (
	ErrLinkedElsewhere = errs.New("that account is already linked to another Hayat account")
	ErrAdminLink       = errs.New("an instance admin's account is never matched by email; sign in with your password and link it from Account")
	ErrNotYourDomain   = errs.New("this company sign-in only signs in addresses at its own verified domains")
	ErrAskForInvite    = errs.New("you don't have an account here yet; ask your admin to invite you")
	ErrLastWayIn       = errs.New("this is your only way to sign in; set a password or link another account first")
)

// Linked is an outside account linked to a Hayat account.
type Linked struct {
	ID         uuid.UUID
	Provider   Provider
	Email      string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// link records that (provider, subject) is userID. The global ref's primary
// key is what refuses the same outside account for two people.
func (f *Service) link(ctx context.Context, u identity.User, p Provider, prof Profile, actor string) error {
	return f.store.Tx(ctx, func(tx pgx.Tx) error {
		id := store.NewID()
		sp, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := sp.Exec(ctx, `INSERT INTO identity_refs (provider_id, subject, user_id, region, identity_id) VALUES ($1, $2, $3, $4, $5)`,
			p.ID, prof.Subject, u.ID, u.Region, id); err != nil {
			_ = sp.Rollback(ctx)
			if store.IsUniqueViolation(err) {
				return ErrLinkedElsewhere
			}
			return err
		}
		if err := sp.Commit(ctx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_identities (region, id, user_id, provider_id, subject, email, last_used_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())`, u.Region, id, u.ID, p.ID, prof.Subject, prof.Email); err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, actor, "user.account_linked", "user:"+u.ID.String(), map[string]any{"provider": p.Name})
	})
}

// SignIn decides which Hayat account a finished trip is, in a fixed order,
// because getting it wrong is how accounts are taken over:
//
//  1. an outside account already linked is that person;
//  2. linking from Account links to whoever is signed in;
//  3. otherwise only an address the provider verified counts, and a
//     company provider only for its tenant's verified domains;
//  4. an existing account with that address is linked — never an instance
//     admin's, and a pending one becomes active (the mailbox just answered);
//  5. someone new gets an account: from Google or GitHub always (sign-up is
//     open), from a company provider only when it creates accounts (JIT),
//     which also adds them to the tenant.
func (f *Service) SignIn(ctx context.Context, r Return) (identity.User, error) {
	p, prof := r.Provider, r.Profile
	actor := "provider:" + p.ID.String()

	var userID uuid.UUID
	var identityID uuid.UUID
	var region string
	err := f.store.Pool.QueryRow(ctx, `SELECT user_id, region, identity_id FROM identity_refs WHERE provider_id = $1 AND subject = $2`, p.ID, prof.Subject).
		Scan(&userID, &region, &identityID)
	switch {
	case err == nil:
		if r.LinkUser != nil && *r.LinkUser != userID {
			return identity.User{}, ErrLinkedElsewhere
		}
		_, _ = f.store.Pool.Exec(ctx, `UPDATE user_identities SET last_used_at = now(), email = $3 WHERE region = $1 AND id = $2`, region, identityID, prof.Email)
		return f.usable(ctx, userID)
	case !errors.Is(err, pgx.ErrNoRows):
		return identity.User{}, err
	}

	if r.LinkUser != nil {
		u, err := f.usable(ctx, *r.LinkUser)
		if err != nil {
			return identity.User{}, err
		}
		return u, f.link(ctx, u, p, prof, "user:"+u.ID.String())
	}

	if !prof.EmailVerified || prof.Email == "" {
		return identity.User{}, ErrNoEmail
	}
	email, err := identity.NormalizeEmail(prof.Email)
	if err != nil {
		return identity.User{}, ErrNoEmail
	}
	if p.TenantID != nil {
		_, d, ok, err := f.CompanySignIn(ctx, email)
		if err != nil {
			return identity.User{}, err
		}
		if !ok || d.TenantID != *p.TenantID {
			return identity.User{}, ErrNotYourDomain
		}
	}

	u, err := f.ids.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		if u.IsAdmin() {
			return identity.User{}, ErrAdminLink
		}
		if u.Status == identity.StatusDisabled {
			return identity.User{}, identity.ErrDisabled
		}
		if !u.EmailVerified || u.Status == identity.StatusPending {
			if u, err = f.ids.ConfirmEmail(ctx, u.ID, actor); err != nil {
				return identity.User{}, err
			}
		}
	case errors.Is(err, store.ErrNotFound):
		if p.TenantID != nil && !p.JIT {
			return identity.User{}, ErrAskForInvite
		}
		if u, err = f.ids.CreateFederated(ctx, email, prof.Name, prof.Picture, actor); err != nil {
			return identity.User{}, err
		}
	default:
		return identity.User{}, err
	}
	if err := f.link(ctx, u, p, prof, actor); err != nil {
		return identity.User{}, err
	}
	if p.TenantID != nil && p.JIT {
		if member, err := f.tenants.IsMember(ctx, *p.TenantID, u.ID); err == nil && !member {
			if err := f.tenants.AddMember(ctx, *p.TenantID, u.ID, actor); err != nil {
				return identity.User{}, err
			}
		}
	}
	return u, nil
}

// usable is the account, refused when it is disabled.
func (f *Service) usable(ctx context.Context, id uuid.UUID) (identity.User, error) {
	u, err := f.ids.GetUser(ctx, id)
	if err != nil {
		return identity.User{}, err
	}
	if u.Status == identity.StatusDisabled {
		return identity.User{}, identity.ErrDisabled
	}
	return u, nil
}

// Linked lists the outside accounts linked to someone, with their providers.
func (f *Service) Linked(ctx context.Context, u identity.User) ([]Linked, error) {
	rows, err := f.store.Pool.Query(ctx, `SELECT id, provider_id, email, created_at, last_used_at FROM user_identities
		WHERE region = $1 AND user_id = $2 ORDER BY created_at`, u.Region, u.ID)
	if err != nil {
		return nil, err
	}
	type row struct {
		l  Linked
		pp uuid.UUID
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.l.ID, &r.pp, &r.l.Email, &r.l.CreatedAt, &r.l.LastUsedAt); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Linked, 0, len(list))
	for _, r := range list {
		p, err := f.Provider(ctx, r.pp)
		if errors.Is(err, store.ErrNotFound) {
			p = Provider{ID: r.pp, Name: "A removed provider"}
		} else if err != nil {
			return nil, err
		}
		r.l.Provider = p
		out = append(out, r.l)
	}
	return out, nil
}

// Unlink removes a linked account, unless it is the last way the person can
// sign in.
func (f *Service) Unlink(ctx context.Context, u identity.User, identityID uuid.UUID) error {
	linked, err := f.Linked(ctx, u)
	if err != nil {
		return err
	}
	var target *Linked
	for i := range linked {
		if linked[i].ID == identityID {
			target = &linked[i]
		}
	}
	if target == nil {
		return store.ErrNotFound
	}
	if !u.HasPassword && len(linked) == 1 {
		return ErrLastWayIn
	}
	return f.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM identity_refs WHERE identity_id = $1`, identityID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_identities WHERE region = $1 AND id = $2`, u.Region, identityID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, "user:"+u.ID.String(), "user.account_unlinked", "user:"+u.ID.String(), map[string]any{"provider": target.Provider.Name})
	})
}

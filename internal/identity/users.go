// Package identity owns accounts, the links emailed to them, and sign-in
// sessions.
//
// Account rules (carried over from tabletsgo, see docs/ARCHITECTURE.md):
//   - A self-registered account can do nothing until its mailbox answers. It is
//     created pending with no password; the password is chosen from the emailed
//     link, never at sign-up.
//   - Sign-up and password reset never reveal whether an address has an account.
//   - Disabling an account or resetting its password ends every session, which
//     also ends every refresh token bound to those sessions.
package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/language"

	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/crypto"
	"github.com/aidapedia/hayat/internal/errs"
	hmail "github.com/aidapedia/hayat/internal/mail"
	"github.com/aidapedia/hayat/internal/revision"
	"github.com/aidapedia/hayat/internal/store"
)

const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusDisabled = "disabled"

	RoleAdmin = "admin"

	MinPasswordLength = 10
	maxPasswordLength = 256

	verifyTTL = 72 * time.Hour
	resetTTL  = time.Hour
)

var (
	ErrBadCredentials = errs.New("the email or password is wrong")
	ErrPending        = errs.New("this account isn't set up yet: open the link we emailed to choose a password")
	ErrDisabled       = errs.New("this account is disabled")
	ErrInvalidEmail   = errs.New("enter a valid email address")
	ErrEmailTaken     = errs.New("an account with this email already exists")
	ErrWeakPassword   = errs.Newf("use at least %d characters for the password", MinPasswordLength)
	ErrLinkInvalid    = errs.New("this link is invalid or has expired: request a new one")
	ErrLastAdmin      = errs.New("this is the last instance admin; promote someone else first")
)

type User struct {
	ID            uuid.UUID
	Region        string
	Email         string
	EmailVerified bool
	Name          string
	AvatarURL     string
	Locale        string
	Zoneinfo      string
	Phone         string
	Status        string
	SystemRole    string
	HasPassword   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (u User) IsAdmin() bool { return u.SystemRole == RoleAdmin }

// DisplayName is the name, or the email when no name was given.
func (u User) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

type Service struct {
	store      *store.Store
	issuer     string
	sessionTTL time.Duration
}

func New(s *store.Store, issuer string, sessionTTL time.Duration) *Service {
	return &Service{store: s, issuer: strings.TrimRight(issuer, "/"), sessionTTL: sessionTTL}
}

const userColumns = `id, region, email, email_verified_at IS NOT NULL, name, avatar_url, locale, zoneinfo, phone,
	status, system_role, password_hash <> '', created_at, updated_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Region, &u.Email, &u.EmailVerified, &u.Name, &u.AvatarURL, &u.Locale, &u.Zoneinfo,
		&u.Phone, &u.Status, &u.SystemRole, &u.HasPassword, &u.CreatedAt, &u.UpdatedAt)
	return u, store.NotFound(err)
}

// NormalizeEmail validates an address and returns it trimmed. Comparison is
// always on the lower-cased form (user_refs.email_lower).
func NormalizeEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw || !strings.Contains(raw, "@") || len(raw) > 254 {
		return "", ErrInvalidEmail
	}
	return raw, nil
}

func checkPassword(p string) error {
	if utf8.RuneCountInString(p) < MinPasswordLength || len(p) > maxPasswordLength {
		return ErrWeakPassword
	}
	return nil
}

// regionOf resolves a user id to its home region through the global ref table.
func (s *Service) regionOf(ctx context.Context, db store.DB, id uuid.UUID) (string, error) {
	var region string
	err := db.QueryRow(ctx, `SELECT region FROM user_refs WHERE id = $1`, id).Scan(&region)
	return region, store.NotFound(err)
}

func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (User, error) {
	return s.getUser(ctx, s.store.Pool, id)
}

func (s *Service) getUser(ctx context.Context, db store.DB, id uuid.UUID) (User, error) {
	region, err := s.regionOf(ctx, db, id)
	if err != nil {
		return User{}, err
	}
	return scanUser(db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE region = $1 AND id = $2`, region, id))
}

func (s *Service) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var id uuid.UUID
	err := s.store.Pool.QueryRow(ctx, `SELECT id FROM user_refs WHERE email_lower = lower($1)`, strings.TrimSpace(email)).Scan(&id)
	if err != nil {
		return User{}, store.NotFound(err)
	}
	return s.GetUser(ctx, id)
}

// GetUsers returns the users with these ids, in no particular order. Unknown
// ids are skipped.
func (s *Service) GetUsers(ctx context.Context, ids []uuid.UUID) ([]User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.store.Pool.Query(ctx, `SELECT `+userColumns+` FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	return collectUsers(rows)
}

func collectUsers(rows pgx.Rows) ([]User, error) {
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListUsers pages through users newest first. query matches email or name.
// The cursor is the created_at of the last row of the previous page.
func (s *Service) ListUsers(ctx context.Context, query string, before time.Time, limit int) ([]User, error) {
	if before.IsZero() {
		before = time.Now().Add(time.Minute)
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.store.Pool.Query(ctx, `SELECT `+userColumns+` FROM users
		WHERE created_at < $1 AND (lower(email) LIKE $2 OR lower(name) LIKE $2)
		ORDER BY created_at DESC LIMIT $3`, before, pattern, limit)
	if err != nil {
		return nil, err
	}
	return collectUsers(rows)
}

func (s *Service) CountUsers(ctx context.Context) (total, active int, err error) {
	err = s.store.Pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE status = 'active') FROM users`).Scan(&total, &active)
	return
}

// HasAdmin reports whether any instance admin exists. Until one does, the
// first-run setup page is open (behind its token).
func (s *Service) HasAdmin(ctx context.Context) (bool, error) {
	var ok bool
	err := s.store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE system_role = 'admin')`).Scan(&ok)
	return ok, err
}

// insertUser writes the account and its global ref. The ref's unique email is
// what makes two sign-ups for one address fail cleanly in any region.
func (s *Service) insertUser(ctx context.Context, tx pgx.Tx, email, name, status, role, passwordHash string, verified bool) (User, error) {
	return s.insertUserID(ctx, tx, store.NewID(), email, name, status, role, passwordHash, verified)
}

func (s *Service) insertUserID(ctx context.Context, tx pgx.Tx, id uuid.UUID, email, name, status, role, passwordHash string, verified bool) (User, error) {
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_refs WHERE email_lower = lower($1))`, email).Scan(&taken); err != nil {
		return User{}, err
	}
	if taken {
		return User{}, ErrEmailTaken
	}
	// A concurrent sign-up can still win the race. The savepoint keeps that
	// unique violation from aborting the caller's transaction.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	if _, err := sp.Exec(ctx, `INSERT INTO user_refs (id, email_lower, region) VALUES ($1, lower($2), $3)`,
		id, email, s.store.Region); err != nil {
		_ = sp.Rollback(ctx)
		if store.IsUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	if err := sp.Commit(ctx); err != nil {
		return User{}, err
	}
	var verifiedAt *time.Time
	if verified {
		now := time.Now()
		verifiedAt = &now
	}
	return scanUser(tx.QueryRow(ctx, `INSERT INTO users (region, id, email, email_verified_at, name, status, system_role, password_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+userColumns,
		s.store.Region, id, email, verifiedAt, strings.TrimSpace(name), status, role, passwordHash))
}

// Register starts a self sign-up. It never says whether the address already
// has an account: a new address gets a link to choose a password, an existing
// one gets a note saying so.
func (s *Service) Register(ctx context.Context, rawEmail, name string) error {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.insertUser(ctx, tx, email, name, StatusPending, "", "", false)
		if errors.Is(err, ErrEmailTaken) {
			existing, err := s.getUserByEmailTx(ctx, tx, email)
			if err != nil {
				return err
			}
			if existing.Status == StatusPending {
				return s.sendVerification(ctx, tx, existing)
			}
			return hmail.Enqueue(ctx, tx, existing.Region, existing.Email, "You already have a Hayat account",
				fmt.Sprintf("Someone tried to create an account with this address, which already has one.\n\nSign in: %s/login\nForgot your password? %s/forgot\n\nIf this wasn't you, you can ignore this email.", s.issuer, s.issuer))
		}
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, u.Region, audit.UserActor(u.ID), "user.registered", "user:"+u.ID.String(), nil); err != nil {
			return err
		}
		return s.sendVerification(ctx, tx, u)
	})
}

// Invite creates a pending account for someone an admin adds, and emails them
// a link to choose a password. An address that already has an account returns
// that account unchanged, with created=false.
func (s *Service) Invite(ctx context.Context, rawEmail, name, actor string) (User, bool, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, false, err
	}
	var out User
	created := false
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.insertUser(ctx, tx, email, name, StatusPending, "", "", false)
		if errors.Is(err, ErrEmailTaken) {
			out, err = s.getUserByEmailTx(ctx, tx, email)
			return err
		}
		if err != nil {
			return err
		}
		out, created = u, true
		if err := audit.Write(ctx, tx, u.Region, actor, "user.invited", "user:"+u.ID.String(), map[string]any{"email": email}); err != nil {
			return err
		}
		return s.sendVerification(ctx, tx, u)
	})
	return out, created, err
}

// ResendVerification emails a fresh link to a pending account.
func (s *Service) ResendVerification(ctx context.Context, id uuid.UUID) error {
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if u.Status != StatusPending {
			return errs.New("only a pending account needs a verification link")
		}
		return s.sendVerification(ctx, tx, u)
	})
}

// CreateAdmin makes the first instance admin from the setup page. The caller
// has already checked the setup token.
func (s *Service) CreateAdmin(ctx context.Context, rawEmail, name, password string) (User, error) {
	return s.CreateActive(ctx, rawEmail, name, password, true, "setup")
}

// CreateActive makes an account that can sign in straight away, with the
// password given. Only an operator reaches it (the setup page and `hayat seed`);
// sign-up and invites stay pending until the mailbox answers.
func (s *Service) CreateActive(ctx context.Context, rawEmail, name, password string, admin bool, actor string) (User, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, err
	}
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		role, event := "", "user.created"
		if admin {
			role, event = RoleAdmin, "user.created_admin"
		}
		u, err = s.insertUser(ctx, tx, email, name, StatusActive, role, hash, false)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, actor, event, "user:"+u.ID.String(), nil)
	})
	return u, err
}

// ImportedUser is an account moved in from another system with `hayat import`.
type ImportedUser struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string // argon2id, or crypto.LegacySHA256Prefix + hex; "" for none
	Status       string // pending | active | disabled
	Zoneinfo     string
	Admin        bool
}

// Import writes an account with the id and password hash it had elsewhere, so
// the other system's references to it stay valid and its owner signs in with
// the password they already know. The address counts as verified only for an
// active account: the other system had already let it in. Only the operator
// reaches this, through `hayat import`.
func (s *Service) Import(ctx context.Context, in ImportedUser, actor string) (User, error) {
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		return User{}, fmt.Errorf("%s: %w", in.Email, err)
	}
	if in.ID == uuid.Nil {
		return User{}, errs.New("an imported account needs its id")
	}
	status := in.Status
	if status != StatusActive && status != StatusDisabled {
		status = StatusPending
	}
	if strings.HasPrefix(in.PasswordHash, "$") && !strings.HasPrefix(in.PasswordHash, "$argon2id$") && !crypto.NeedsRehash(in.PasswordHash) {
		return User{}, errs.Newf("%s: unknown password hash format", email)
	}
	role := ""
	if in.Admin {
		role = RoleAdmin
	}
	var u User
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		if u, err = s.insertUserID(ctx, tx, in.ID, email, in.Name, status, role, in.PasswordHash, status == StatusActive); err != nil {
			return err
		}
		if in.Zoneinfo != "" {
			if u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET zoneinfo = $3 WHERE region = $1 AND id = $2 RETURNING `+userColumns,
				u.Region, u.ID, in.Zoneinfo)); err != nil {
				return err
			}
		}
		if in.Admin {
			if _, err := revision.Bump(ctx, tx, u.Region, revision.Global, "user.imported_admin"); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, u.Region, actor, "user.imported", "user:"+u.ID.String(), map[string]any{"email": email})
	})
	return u, err
}

func (s *Service) getUserByEmailTx(ctx context.Context, db store.DB, email string) (User, error) {
	var id uuid.UUID
	if err := db.QueryRow(ctx, `SELECT id FROM user_refs WHERE email_lower = lower($1)`, email).Scan(&id); err != nil {
		return User{}, store.NotFound(err)
	}
	return s.getUser(ctx, db, id)
}

// ---------- Emailed links ----------

func (s *Service) newEmailToken(ctx context.Context, tx pgx.Tx, u User, purpose string, ttl time.Duration) (string, error) {
	// Only the newest link of a purpose works.
	if _, err := tx.Exec(ctx, `UPDATE email_tokens SET used_at = now()
		WHERE region = $1 AND user_id = $2 AND purpose = $3 AND used_at IS NULL`, u.Region, u.ID, purpose); err != nil {
		return "", err
	}
	token := crypto.NewToken()
	_, err := tx.Exec(ctx, `INSERT INTO email_tokens (region, id, user_id, purpose, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, u.Region, store.NewID(), u.ID, purpose, crypto.HashToken(token), time.Now().Add(ttl))
	return token, err
}

// link carries the region so the token is looked up in the user's home region.
func (s *Service) link(path, region, token string) string {
	return fmt.Sprintf("%s%s?r=%s&t=%s", s.issuer, path, region, token)
}

func (s *Service) sendVerification(ctx context.Context, tx pgx.Tx, u User) error {
	token, err := s.newEmailToken(ctx, tx, u, "verify", verifyTTL)
	if err != nil {
		return err
	}
	return hmail.Enqueue(ctx, tx, u.Region, u.Email, "Finish setting up your Hayat account",
		fmt.Sprintf("Hi %s,\n\nChoose a password to finish setting up your account:\n%s\n\nThis link works once and expires in 3 days.", u.DisplayName(), s.link("/verify", u.Region, token)))
}

// consumeEmailToken marks a link used and returns its owner, in one transaction
// with whatever the caller does next.
func (s *Service) consumeEmailToken(ctx context.Context, tx pgx.Tx, region, token, purpose string) (User, error) {
	var userID uuid.UUID
	err := tx.QueryRow(ctx, `UPDATE email_tokens SET used_at = now()
		WHERE region = $1 AND token_hash = $2 AND purpose = $3 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`, region, crypto.HashToken(token), purpose).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrLinkInvalid
		}
		return User{}, err
	}
	return s.getUser(ctx, tx, userID)
}

// CheckLink reports whether a link is still usable, without using it. The
// pages call it before showing the form.
func (s *Service) CheckLink(ctx context.Context, region, token, purpose string) (User, error) {
	var userID uuid.UUID
	err := s.store.Pool.QueryRow(ctx, `SELECT user_id FROM email_tokens
		WHERE region = $1 AND token_hash = $2 AND purpose = $3 AND used_at IS NULL AND expires_at > now()`,
		region, crypto.HashToken(token), purpose).Scan(&userID)
	if err != nil {
		return User{}, ErrLinkInvalid
	}
	return s.GetUser(ctx, userID)
}

// CompleteVerification is the emailed link of a pending account: the person
// proves the mailbox and chooses a password in one step.
func (s *Service) CompleteVerification(ctx context.Context, region, token, password string) (User, error) {
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		owner, err := s.consumeEmailToken(ctx, tx, region, token, "verify")
		if err != nil {
			return err
		}
		if owner.Status == StatusDisabled {
			return ErrDisabled
		}
		u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET password_hash = $3, status = 'active',
			email_verified_at = coalesce(email_verified_at, now()), updated_at = now()
			WHERE region = $1 AND id = $2 RETURNING `+userColumns, owner.Region, owner.ID, hash))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, audit.UserActor(u.ID), "user.verified", "user:"+u.ID.String(), nil)
	})
	return u, err
}

// RequestPasswordReset emails a reset link to an active account. Like Register,
// it gives the same answer whether or not the address exists.
func (s *Service) RequestPasswordReset(ctx context.Context, rawEmail string) error {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUserByEmailTx(ctx, tx, email)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		switch u.Status {
		case StatusPending:
			return s.sendVerification(ctx, tx, u)
		case StatusDisabled:
			return nil
		}
		token, err := s.newEmailToken(ctx, tx, u, "reset", resetTTL)
		if err != nil {
			return err
		}
		return hmail.Enqueue(ctx, tx, u.Region, u.Email, "Reset your Hayat password",
			fmt.Sprintf("Hi %s,\n\nChoose a new password here:\n%s\n\nThis link works once and expires in 1 hour. If you didn't ask for it, ignore this email; your password stays the same.", u.DisplayName(), s.link("/reset", u.Region, token)))
	})
}

// ResetPassword sets a new password from an emailed link and signs the account
// out everywhere.
func (s *Service) ResetPassword(ctx context.Context, region, token, password string) (User, error) {
	if err := checkPassword(password); err != nil {
		return User{}, err
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.store.Tx(ctx, func(tx pgx.Tx) error {
		owner, err := s.consumeEmailToken(ctx, tx, region, token, "reset")
		if err != nil {
			return err
		}
		if owner.Status != StatusActive {
			return ErrLinkInvalid
		}
		u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET password_hash = $3, updated_at = now()
			WHERE region = $1 AND id = $2 RETURNING `+userColumns, owner.Region, owner.ID, hash))
		if err != nil {
			return err
		}
		if err := revokeAllSessions(ctx, tx, u.Region, u.ID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, audit.UserActor(u.ID), "user.password_reset", "user:"+u.ID.String(), nil)
	})
	return u, err
}

// ---------- Sign-in and account changes ----------

// Authenticate checks an email and password. An unknown email costs the same
// time as a wrong password, so timing doesn't reveal which addresses exist.
func (s *Service) Authenticate(ctx context.Context, email, password string) (User, error) {
	u, err := s.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		crypto.BurnPasswordTime(password)
		return User{}, ErrBadCredentials
	}
	if err != nil {
		return User{}, err
	}
	var hash string
	if err := s.store.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE region = $1 AND id = $2`, u.Region, u.ID).Scan(&hash); err != nil {
		return User{}, err
	}
	if u.Status == StatusPending {
		crypto.BurnPasswordTime(password)
		return User{}, ErrPending
	}
	if !crypto.VerifyPassword(hash, password) {
		return User{}, ErrBadCredentials
	}
	if u.Status == StatusDisabled {
		return User{}, ErrDisabled
	}
	if crypto.NeedsRehash(hash) {
		// An imported hash: swap it for argon2id now that we hold the password.
		// A failure here costs nothing but the upgrade, so sign-in goes ahead.
		if fresh, err := crypto.HashPassword(password); err == nil {
			_, _ = s.store.Pool.Exec(ctx, `UPDATE users SET password_hash = $3, updated_at = now() WHERE region = $1 AND id = $2 AND password_hash = $4`,
				u.Region, u.ID, fresh, hash)
		}
	}
	return u, nil
}

func (s *Service) ChangePassword(ctx context.Context, id uuid.UUID, current, next string, keepSession uuid.UUID) error {
	if err := checkPassword(next); err != nil {
		return err
	}
	hash, err := crypto.HashPassword(next)
	if err != nil {
		return err
	}
	return s.store.Tx(ctx, func(tx pgx.Tx) error {
		u, err := s.getUser(ctx, tx, id)
		if err != nil {
			return err
		}
		var old string
		if err := tx.QueryRow(ctx, `SELECT password_hash FROM users WHERE region = $1 AND id = $2`, u.Region, u.ID).Scan(&old); err != nil {
			return err
		}
		if old != "" && !crypto.VerifyPassword(old, current) {
			return errs.New("the current password is wrong")
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $3, updated_at = now() WHERE region = $1 AND id = $2`, u.Region, u.ID, hash); err != nil {
			return err
		}
		// Everyone else holding a session for this account is signed out; the
		// person changing the password stays signed in where they are.
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now()
			WHERE region = $1 AND user_id = $2 AND revoked_at IS NULL AND id <> $3`, u.Region, u.ID, keepSession); err != nil {
			return err
		}
		return audit.Write(ctx, tx, u.Region, audit.UserActor(u.ID), "user.password_changed", "user:"+u.ID.String(), nil)
	})
}

type Profile struct {
	Name     string
	Locale   string
	Zoneinfo string
	Phone    string
}

func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, p Profile, actor string) (User, error) {
	if len(p.Name) > 200 || len(p.Locale) > 35 || len(p.Zoneinfo) > 64 || len(p.Phone) > 40 {
		return User{}, errs.New("one of the fields is too long")
	}
	if p.Zoneinfo != "" {
		if _, err := time.LoadLocation(p.Zoneinfo); err != nil {
			return User{}, errs.Newf("%q is not a known time zone", p.Zoneinfo)
		}
	}
	if p.Locale = strings.TrimSpace(p.Locale); p.Locale != "" {
		tag, err := language.Parse(p.Locale)
		if err != nil {
			return User{}, errs.Newf("%q is not a language tag, such as en or id-ID", p.Locale)
		}
		p.Locale = tag.String()
	}
	if p.Phone = normalizePhone(p.Phone); p.Phone != "" && !e164.MatchString(p.Phone) {
		return User{}, errs.New("the phone number needs its country code, such as +62 812 3456 7890")
	}
	var u User
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		region, err := s.regionOf(ctx, tx, id)
		if err != nil {
			return err
		}
		u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET name = $3, locale = $4, zoneinfo = $5, phone = $6, updated_at = now()
			WHERE region = $1 AND id = $2 RETURNING `+userColumns,
			region, id, strings.TrimSpace(p.Name), strings.TrimSpace(p.Locale), strings.TrimSpace(p.Zoneinfo), strings.TrimSpace(p.Phone)))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, region, actor, "user.profile_updated", "user:"+id.String(), nil)
	})
	return u, err
}

// e164 is what the OIDC phone_number claim should carry: +, then 7 to 15 digits.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// normalizePhone drops the spaces, dashes, dots and brackets people type.
func normalizePhone(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')', '\u00a0':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// SetStatus disables or re-enables an account. Disabling ends every session.
// A pending account can't be enabled by hand: it becomes active when its
// owner proves the mailbox.
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, disabled bool, actor string) (User, error) {
	var u User
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := s.getUser(ctx, tx, id)
		if err != nil {
			return err
		}
		next := StatusActive
		if disabled {
			next = StatusDisabled
			if cur.IsAdmin() {
				if err := s.ensureAnotherAdmin(ctx, tx, id); err != nil {
					return err
				}
			}
		} else if !cur.HasPassword {
			next = StatusPending
		}
		u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET status = $3, updated_at = now()
			WHERE region = $1 AND id = $2 RETURNING `+userColumns, cur.Region, id, next))
		if err != nil {
			return err
		}
		if disabled {
			if err := revokeAllSessions(ctx, tx, cur.Region, id); err != nil {
				return err
			}
		}
		// Cached decisions don't carry the account's status; a global bump
		// makes every cache re-read it.
		if _, err := revision.Bump(ctx, tx, cur.Region, revision.Global, "user.status_changed"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, cur.Region, actor, "user.status_changed", "user:"+id.String(), map[string]any{"status": next})
	})
	return u, err
}

// SetSystemRole promotes an account to instance admin or demotes it. The last
// admin can't be demoted, or nobody could administer the instance.
func (s *Service) SetSystemRole(ctx context.Context, id uuid.UUID, admin bool, actor string) (User, error) {
	role := ""
	if admin {
		role = RoleAdmin
	}
	var u User
	err := s.store.Tx(ctx, func(tx pgx.Tx) error {
		cur, err := s.getUser(ctx, tx, id)
		if err != nil {
			return err
		}
		if !admin && cur.IsAdmin() {
			if err := s.ensureAnotherAdmin(ctx, tx, id); err != nil {
				return err
			}
		}
		if admin && cur.Status != StatusActive {
			return errs.New("only an active account can become an instance admin")
		}
		u, err = scanUser(tx.QueryRow(ctx, `UPDATE users SET system_role = $3, updated_at = now()
			WHERE region = $1 AND id = $2 RETURNING `+userColumns, cur.Region, id, role))
		if err != nil {
			return err
		}
		if _, err := revision.Bump(ctx, tx, cur.Region, revision.Global, "user.role_changed"); err != nil {
			return err
		}
		return audit.Write(ctx, tx, cur.Region, actor, "user.role_changed", "user:"+id.String(), map[string]any{"system_role": role})
	})
	return u, err
}

func (s *Service) ensureAnotherAdmin(ctx context.Context, tx pgx.Tx, except uuid.UUID) error {
	var others int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE system_role = 'admin' AND status = 'active' AND id <> $1`, except).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return ErrLastAdmin
	}
	return nil
}
